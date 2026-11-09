package packcopy_world

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

const (
	// walkChunkLen is the nodes or entries one state file chunk holds.
	walkChunkLen = 4096
	// walkStateMsgMax bounds one message of a state file, so a damaged length
	// fails the read instead of allocating it.
	walkStateMsgMax = 64 << 20
	// walkStateTmp is the suffix of the file a save writes before renaming it
	// into place.
	walkStateTmp = ".tmp"
)

// ReadWalkState reads the header of the walk state file at path: the pin, the
// phase and the counts as of the last save.
func ReadWalkState(path string) (*WalkState, error) {
	// Open the file.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read its first message, which names the pin.
	state := &WalkState{}
	if err := readWalkMsg(bufio.NewReader(f), state); err != nil {
		return nil, errors.Wrap(err, "read the walk state header")
	}
	if len(state.GetPin().GetHeads()) == 0 {
		return nil, errors.New("the walk state names no pin")
	}
	return state, nil
}

// DiscardWalkState removes the walk state file at path and any save left
// unfinished beside it. A missing file is not an error.
func DiscardWalkState(path string) error {
	for _, p := range []string{path + walkStateTmp, path} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// walkMsg is a message a state file holds.
type walkMsg interface {
	// SizeVT returns the message's encoded size.
	SizeVT() int
	// MarshalToSizedBufferVT encodes the message into the end of buf.
	MarshalToSizedBufferVT(buf []byte) (int, error)
	// UnmarshalVT decodes the message.
	UnmarshalVT(data []byte) error
}

// writeWalkMsg writes msg with its uvarint length before it, reusing buf, and
// returns buf for the next message.
func writeWalkMsg(w io.Writer, buf []byte, msg walkMsg) ([]byte, error) {
	// Encode the length, then the message after it.
	n := msg.SizeVT()
	buf = binary.AppendUvarint(buf[:0], uint64(n)) //nolint:gosec // a size is not negative.
	head := len(buf)
	buf = append(buf, make([]byte, n)...)
	if _, err := msg.MarshalToSizedBufferVT(buf[head:]); err != nil {
		return buf, err
	}

	// Write both.
	_, err := w.Write(buf)
	return buf, err
}

// readWalkMsg reads one length-prefixed message into msg.
func readWalkMsg(r *bufio.Reader, msg walkMsg) error {
	// Read the length, refusing one too large to be a message.
	n, err := binary.ReadUvarint(r)
	if err != nil {
		return err
	}
	if n > walkStateMsgMax {
		return errors.Errorf("walk state message of %d bytes", n)
	}

	// Read and decode the message.
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}
	return msg.UnmarshalVT(data)
}

// frontier is a copy of a walk's position for a save.
type frontier struct {
	// state is the header, its counts set.
	state *WalkState
	// items are the blocks the walk will read.
	items []walkItem
}

// takeFrontier copies the walk's position: the queued, held, retried and
// running reads, and the counts. A running read stays running until it has
// queued each of its children, so the copy reaches every block left to read.
// A done walk has none.
func (w *Walk) takeFrontier() *frontier {
	// Copy the header and the counts.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	state := w.state.CloneVT()
	state.Status = w.status.CloneVT()
	st := state.Status

	// Add the queue sizes; no read runs in a saved position.
	st.Visited = w.visitedBefore + uint64(len(w.visited))
	st.Queued = uint64(len(w.stacks[0]) + len(w.stacks[1]) + len(w.held))
	st.RetryPending = uint64(len(w.retry))
	st.InFlight, st.BlocksPerSec = 0, 0

	// A done walk has no frontier.
	if state.GetPhase() == WalkPhase_WALK_PHASE_DONE {
		return &frontier{state: state}
	}

	// Copy every block the walk will read.
	items := make([]walkItem, 0, len(w.stacks[0])+len(w.stacks[1])+len(w.retry)+len(w.held)+len(w.flying))
	items = append(items, w.stacks[0]...)
	items = append(items, w.stacks[1]...)
	items = append(items, w.retry...)
	items = append(items, w.held...)
	for _, it := range w.flying {
		items = append(items, it)
	}
	return &frontier{state: state, items: items}
}

// frontierEncoder encodes a frontier's blocks for a state file: each block's
// type, and the blocks that named them, each node after its parent.
type frontierEncoder struct {
	// types saves the block types.
	types *typeSaver
	// ids numbers the nodes from 1.
	ids map[*walkNode]uint64
	// nodes are the numbered nodes in order.
	nodes []*WalkNode
	// entries are the encoded blocks.
	entries []*WalkEntry
}

// node numbers n after its parents, returning 0 for none.
func (e *frontierEncoder) node(n *walkNode) (uint64, error) {
	// A node is numbered once.
	if n == nil {
		return 0, nil
	}
	if id, ok := e.ids[n]; ok {
		return id, nil
	}

	// Number its parent first, then save it with its type.
	parent, err := e.node(n.parent)
	if err != nil {
		return 0, err
	}
	typ, err := e.types.save(n.typ)
	if err != nil {
		return 0, err
	}
	e.nodes = append(e.nodes, &WalkNode{Ref: n.ref, Parent: parent, Stage: uint32(n.stage), Type: typ})
	e.ids[n] = uint64(len(e.nodes))
	return uint64(len(e.nodes)), nil
}

// entry encodes a block the walk will read.
func (e *frontierEncoder) entry(it walkItem) error {
	// Number the block that named it and save its type.
	parent, err := e.node(it.parent)
	if err != nil {
		return err
	}
	typ, err := e.types.save(it.typ)
	if err != nil {
		return err
	}

	// Record it.
	e.entries = append(e.entries, &WalkEntry{
		Ref:       it.ref,
		Parent:    parent,
		Stage:     uint32(it.stage),
		OkraValue: it.okraValue,
		Type:      typ,
	})
	return nil
}

// persist saves the walk's position to the state file: it writes the whole
// file beside the path, syncs it, renames it into place and syncs the
// directory, so the path always holds one complete position. A walk with no
// StatePath saves nothing.
func (w *Walk) persist() error {
	// One save runs at a time.
	if w.conf.StatePath == "" {
		return nil
	}
	w.persistMtx.Lock()
	defer w.persistMtx.Unlock()
	start := time.Now()

	// Encode the position's blocks with their types and parents.
	fr := w.takeFrontier()
	enc := &frontierEncoder{types: newTypeSaver(w.reg), ids: make(map[*walkNode]uint64)}
	for _, it := range fr.items {
		if err := enc.entry(it); err != nil {
			return err
		}
	}

	// Fill in the header.
	fr.state.Ctors = enc.types.ctorList
	fr.state.Xfrms = enc.types.xfrmList
	fr.state.Nodes = uint64(len(enc.nodes))
	fr.state.Entries = uint64(len(enc.entries))
	fr.state.PersistedAt = time.Now().UnixMilli()
	fr.state.Status.PersistedAt = fr.state.PersistedAt
	fr.state.Status.Frontier = fr.state.Entries

	// Write the file beside the path, then rename it into place.
	size, err := writeWalkState(w.conf.StatePath, fr.state, enc.nodes, enc.entries)
	if err != nil {
		return errors.Wrap(err, "save the walk position")
	}

	// Record the save.
	w.mtx.Lock()
	w.status.PersistedAt = fr.state.PersistedAt
	w.status.Frontier = fr.state.Entries
	w.status.StateBytes = uint64(size) //nolint:gosec // a size is not negative.
	w.persistTook = time.Since(start)
	w.mtx.Unlock()
	if w.persisted != nil {
		w.persisted()
	}
	return nil
}

// writeWalkState writes a state file to path through a file beside it: the
// header, then the nodes and the entries in chunks. It syncs the file, renames
// it into place and syncs the directory, and returns the file's size.
func writeWalkState(path string, state *WalkState, nodes []*WalkNode, entries []*WalkEntry) (int64, error) {
	// Write the whole position to the file beside the path and sync it.
	tmp := path + walkStateTmp
	size, err := writeWalkFile(tmp, state, nodes, entries)
	if err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}

	// Rename it into place.
	if err := os.Rename(tmp, path); err != nil {
		return 0, err
	}

	// Sync the directory that names it.
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return 0, err
	}
	err = dir.Sync()
	if cerr := dir.Close(); err == nil {
		err = cerr
	}
	return size, err
}

// writeWalkFile writes a state file at path and syncs it, returning its size.
func writeWalkFile(path string, state *WalkState, nodes []*WalkNode, entries []*WalkEntry) (int64, error) {
	// Create the file.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// Write the header, then the nodes and the entries in chunks.
	bw := bufio.NewWriterSize(f, 1<<20)
	buf, err := writeWalkMsg(bw, nil, state)
	for i := 0; err == nil && i < len(nodes); i += walkChunkLen {
		buf, err = writeWalkMsg(bw, buf, &WalkChunk{Nodes: nodes[i:min(i+walkChunkLen, len(nodes))]})
	}
	for i := 0; err == nil && i < len(entries); i += walkChunkLen {
		buf, err = writeWalkMsg(bw, buf, &WalkChunk{Entries: entries[i:min(i+walkChunkLen, len(entries))]})
	}
	if err != nil {
		return 0, err
	}

	// Flush and sync it.
	if err := bw.Flush(); err != nil {
		return 0, err
	}
	if err := f.Sync(); err != nil {
		return 0, err
	}
	size, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	return size, f.Close()
}

// persistLoop saves the position every PersistEvery, or longer when a save
// was slow, until ctx ends or the returned stop is called; stop waits for a
// save in progress.
func (w *Walk) persistLoop(ctx context.Context) func() {
	// A walk with no state file saves nothing.
	if w.conf.StatePath == "" {
		return func() {}
	}

	// Save until stopped.
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			// Wait the interval, longer after a slow save.
			w.mtx.Lock()
			wait := max(w.conf.PersistEvery, walkPersistShare*w.persistTook)
			w.mtx.Unlock()
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}

			// Save, logging a failure; the next save tries again.
			if err := w.persist(); err != nil {
				w.le.WithError(err).Warn("World block walk: save the position")
			}
		}
	})

	// Stop by cancelling and waiting for the loop.
	return func() {
		cancel()
		wg.Wait()
	}
}

// loadNode is a saved node as a resume rebuilds it.
type loadNode struct {
	// saved is the node as saved.
	saved *WalkNode
	// parent is the node that named it, nil for none.
	parent *loadNode
	// node is the rebuilt node.
	node *walkNode
	// typ is the node's type, nil when unknown.
	typ any
	// typed marks a node whose type is known without its parent: saved, or a
	// pinned root's.
	typed bool
	// need marks a node read again to type a block it names.
	need bool
	// kids types the node's children by ref, nil for none known.
	kids map[string]any
	// gone marks a needed node that could not be read again, or whose
	// ancestor could not; the walk reads the first such node again.
	gone bool
	// depth counts the needed nodes above this one.
	depth int
}

// loadEntry is a saved entry as a resume rebuilds it.
type loadEntry struct {
	// saved is the entry as saved.
	saved *WalkEntry
	// parent is the node that named it, nil for none.
	parent *loadNode
	// typ is the entry's type, when typed.
	typ any
	// typed marks an entry whose type is known without its parent.
	typed bool
}

// load reads the saved frontier and queues it. A block saved with its type
// is queued as saved; a block whose type was not saved is typed by reading
// its parent again and decoding it, and a parent it cannot read is queued
// itself, so the walk reads its subtree again.
func (w *Walk) load(ctx context.Context) error {
	// Read the saved header.
	f, err := os.Open(w.conf.StatePath)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	header := &WalkState{}
	if err := readWalkMsg(r, header); err != nil {
		return err
	}

	// Read the saved nodes and entries.
	var nodes []*loadNode
	var entries []*loadEntry
	for uint64(len(nodes)) < header.GetNodes() || uint64(len(entries)) < header.GetEntries() {
		chunk := &WalkChunk{}
		if err := readWalkMsg(r, chunk); err != nil {
			return err
		}
		for _, n := range chunk.GetNodes() {
			nodes = append(nodes, &loadNode{saved: n})
		}
		for _, e := range chunk.GetEntries() {
			entries = append(entries, &loadEntry{saved: e})
		}
	}

	// Link each node and entry to its parent, and rebuild the saved types.
	types := &typeLoader{typer: w.typer, reg: w.reg, ctors: header.GetCtors(), xfrms: header.GetXfrms(), built: make(map[uint32]xfrmPair)}
	parentOf := func(id uint64, limit int) (*loadNode, error) {
		if id == 0 {
			return nil, nil
		}
		if id > uint64(limit) { //nolint:gosec // a count is not negative.
			return nil, errors.Errorf("walk state names node %d of %d", id, limit)
		}
		return nodes[id-1], nil
	}
	for i, n := range nodes {
		if n.parent, err = parentOf(n.saved.GetParent(), i); err != nil {
			return err
		}
		n.typ, n.typed = types.load(n.saved.GetType())
		if !n.typed && n.parent == nil {
			n.typ, n.typed = w.typer.RootType(n.saved.GetRef()), true
		}
	}
	for _, e := range entries {
		if e.parent, err = parentOf(e.saved.GetParent(), len(nodes)); err != nil {
			return err
		}
		e.typ, e.typed = types.load(e.saved.GetType())
		if !e.typed && e.parent == nil {
			e.typ, e.typed = w.typer.RootType(e.saved.GetRef()), true
		}
		if !e.typed {
			e.parent.need = true
		}
	}

	// Read again each node a block's type needs, from the roots down.
	if err := w.typeNodes(ctx, nodes); err != nil {
		return err
	}

	// Queue each first gone node, then each entry under no gone node.
	for _, n := range nodes {
		if n.gone && (n.parent == nil || !n.parent.gone) {
			w.push(w.loadItem(n.parent, n.saved.GetRef(), n.typ, uint8(n.saved.GetStage()), false)) //nolint:gosec // a stage is 0 or 1.
		}
	}
	for _, e := range entries {
		if p := e.parent; p != nil && p.gone {
			continue
		}
		typ := e.typ
		if !e.typed {
			typ = e.parent.kids[e.saved.GetRef().MarshalString()]
		}
		w.push(w.loadItem(e.parent, e.saved.GetRef(), typ, uint8(e.saved.GetStage()), e.saved.GetOkraValue())) //nolint:gosec // a stage is 0 or 1.
	}

	// Count the blocks earlier runs reached that this run did not queue.
	w.mtx.Lock()
	if v, q := w.status.GetVisited(), uint64(len(w.visited)); v > q {
		w.visitedBefore = v - q
	}
	w.le.WithField("nodes", len(nodes)).
		WithField("entries", len(entries)).
		WithField("phase", w.state.GetPhase().String()).
		WithField("resumes", w.state.GetResumes()).
		Info("World block walk: resumed the saved position")
	w.mtx.Unlock()
	return nil
}

// loadItem builds the item of a saved block named by p.
func (w *Walk) loadItem(p *loadNode, ref *block.BlockRef, typ any, stage uint8, okraValue bool) walkItem {
	it := walkItem{ref: ref, typ: typ, stage: min(stage, walkStageObjects), okraValue: okraValue}
	if p != nil {
		it.parent = p.node
	}
	return it
}

// typeNodes reads again the saved nodes a block's type needs, from the roots
// down one depth at a time and many reads at once, and types each child of
// each by its decode. It rebuilds every node.
func (w *Walk) typeNodes(ctx context.Context, nodes []*loadNode) error {
	// A needed node typed by its parent needs its parent too.
	for i := len(nodes) - 1; i >= 0; i-- {
		if n := nodes[i]; n.need && !n.typed {
			n.parent.need = true
		}
	}

	// Rebuild each node, and group the needed nodes by depth.
	var levels [][]*loadNode
	for _, n := range nodes {
		n.node = &walkNode{ref: n.saved.GetRef(), stage: uint8(min(n.saved.GetStage(), walkStageObjects))} //nolint:gosec // a stage is 0 or 1.
		if !n.typed && n.parent != nil {
			n.node.parent = n.parent.node
		}
		if !n.need {
			n.node.typ = n.typ
			continue
		}
		if !n.typed {
			n.depth = n.parent.depth + 1
		}
		if n.depth == len(levels) {
			levels = append(levels, nil)
		}
		levels[n.depth] = append(levels[n.depth], n)
	}

	// Read and decode each depth's nodes, many at once.
	for _, level := range levels {
		var wg sync.WaitGroup
		work := make(chan *loadNode)
		for range min(w.conc, len(level)) {
			wg.Go(func() {
				for n := range work {
					w.typeNode(ctx, n)
				}
			})
		}
		for _, n := range level {
			work <- n
		}
		close(work)
		wg.Wait()
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
	}
	return nil
}

// typeNode types one needed node, by its parent's decode unless its type is
// known, then reads it again and decodes it to type its children. A node it
// cannot read is gone.
func (w *Walk) typeNode(ctx context.Context, n *loadNode) {
	// Take the node's type from its parent unless it is known.
	if !n.typed {
		if n.parent.gone {
			n.gone = true
			return
		}
		n.typ = n.parent.kids[n.saved.GetRef().MarshalString()]
	}
	n.node.typ = n.typ
	if n.typ == nil {
		return
	}

	// Read it again and decode its children's types.
	rctx, cancel := context.WithTimeout(ctx, w.conf.ReadTimeout)
	stored, err := w.conf.Source.GetStoredBlock(rctx, n.saved.GetRef())
	cancel()
	if err != nil || stored == nil {
		n.gone = true
		return
	}
	kids, ok, err := w.typer.Decode(ctx, n.typ, n.saved.GetRef(), stored.Data)
	if err != nil || !ok {
		return
	}
	n.kids = make(map[string]any, len(kids))
	for _, kid := range kids {
		if kid.Type != nil {
			n.kids[kid.Ref.MarshalString()] = kid.Type
		}
	}
}
