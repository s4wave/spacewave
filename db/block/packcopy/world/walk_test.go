package packcopy_world

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

// walkHiddenRefs serves a store's blocks with their refs unknown, as a peer
// that never recorded them does, and misses one block. Each read is counted
// when it returns to a walk still running; hook, when set, runs first and may
// fail the read.
type walkHiddenRefs struct {
	store block.StoreOps
	miss  walkKey
	hook  func(ctx context.Context, ref *block.BlockRef) error
	// mtx guards reads.
	mtx sync.Mutex
	// reads counts the reads of each block.
	reads map[walkKey]int
}

// GetStoredBlock returns the block's bytes alone, or a miss.
func (h *walkHiddenRefs) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if h.hook != nil {
		if err := h.hook(ctx, ref); err != nil {
			return nil, err
		}
	}
	k := keyOf(ref)
	if ctx.Err() == nil {
		h.mtx.Lock()
		h.reads[k]++
		h.mtx.Unlock()
	}
	if k == h.miss {
		return nil, nil
	}
	data, found, err := h.store.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.StoredBlock{Data: data}, nil
}

// readSet returns the blocks read.
func (h *walkHiddenRefs) readSet() map[walkKey]int {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	out := make(map[walkKey]int, len(h.reads))
	for k, n := range h.reads {
		out[k] = n
	}
	return out
}

// walkKept records the blocks a walk stores with decoded refs.
type walkKept struct {
	mtx  sync.Mutex
	refs map[walkKey]int
}

// StoreDecodedRefs records the block and its ref count.
func (k *walkKept) StoreDecodedRefs(_ context.Context, ref *block.BlockRef, _ []byte, refs []*block.BlockRef) error {
	k.mtx.Lock()
	k.refs[keyOf(ref)] = len(refs)
	k.mtx.Unlock()
	return nil
}

// walkWorld is an encrypted test World.
type walkWorld struct {
	tb     *testbed.Testbed
	le     *logrus.Entry
	engine *world_block.Engine
	ws     world.WorldState
	xfrm   *block_transform.Config
}

// newWalkWorld builds an encrypted World of objects typed test/blob, each
// with a blob root of size bytes, and quads graph quads, so the graph tree
// spans several okra pages.
func newWalkWorld(t *testing.T, objects, size, quads int) *walkWorld {
	// Encrypt every block, so only a typed decode reads one.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	le := logrus.NewEntry(log)
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	xfrm, err := block_transform.NewConfig([]config.Config{&transform_blockenc.Config{
		BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
		Key:      key,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cursor, _, err := bucket_lookup.BuildEmptyCursor(ctx, tb.Bus, le, tb.StepFactorySet, tb.BucketId, tb.Volume.GetID(), xfrm, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cursor.Release)
	engine, err := world_block.NewEngine(ctx, le, cursor, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	// Write the objects and the quads.
	ww := &walkWorld{tb: tb, le: le, engine: engine, ws: world.NewEngineWorldState(engine, true), xfrm: xfrm}
	for i := range objects {
		ww.addObject(t, "obj-"+strconv.Itoa(i), size)
	}
	for i := range quads {
		q := world.NewGraphQuadWithKeys("obj-"+strconv.Itoa(i%objects), "<link-"+strconv.Itoa(i)+">", "obj-"+strconv.Itoa((i+1)%objects), "")
		if err := ww.ws.SetGraphQuad(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	return ww
}

// addObject writes an object typed test/blob with a blob root of size bytes.
func (ww *walkWorld) addObject(t *testing.T, key string, size int) {
	ctx := t.Context()
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	obj, _, err := world.CreateWorldObject(ctx, ww.ws, key, func(bcs *block.Cursor) error {
		_, err := blob.BuildBlobWithBytes(ctx, data, bcs)
		return err
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, ww.ws, key, "test/blob"); err != nil {
		t.Fatal(err)
	}
}

// head returns the World's current head with its transform.
func (ww *walkWorld) head() *bucket.ObjectRef {
	head := ww.engine.GetRootRef()
	head.TransformConf = ww.xfrm
	return head
}

// walkPinOf returns the pin of head.
func walkPinOf(head *bucket.ObjectRef, seqno uint64) *WalkPin {
	return &WalkPin{Heads: []*bucket.ObjectRef{head}, Seqno: seqno}
}

// conf returns a walk config reading src, typing test/blob objects.
func (ww *walkWorld) conf(src WalkSource, conc int) WalkConfig {
	return WalkConfig{
		Source:      src,
		Store:       ww.tb.Volume,
		RootTypes:   RootTypes{"test/blob": blob.NewBlobBlock},
		Concurrency: conc,
	}
}

// objectRoot returns the root block of the object at key in the head.
func (ww *walkWorld) objectRoot(t *testing.T, head *bucket.ObjectRef, key string) *block.BlockRef {
	t.Helper()
	ctx := t.Context()
	snap, err := world_block.OpenSnapshot(ctx, ww.le, ww.tb.Volume, head)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	tx, err := snap.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	obj, found, err := tx.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil || !found {
		t.Fatalf("object %s: found=%t %v", key, found, err)
	}
	root, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return root.GetRootRef()
}

// TestWalkHiddenRefs walks an encrypted World whose every block is read
// without its refs, its graph tree spanning several okra pages, with one
// block missing. The walk reads every block the store holds under the head
// once, keeps each with its decoded refs, reads every okra page, and records
// the missing block as its one hole.
func TestWalkHiddenRefs(t *testing.T) {
	ctx := t.Context()
	const objects, quads = 16, 1500
	ww := newWalkWorld(t, objects, 300<<10, quads)
	head := ww.head()

	// Miss the first object's root, a blob of several chunks.
	missRef := ww.objectRoot(t, head, "obj-0")
	leaf := keyOf(missRef)

	// Walk with every ref hidden, typed by the World.
	src := &walkHiddenRefs{store: ww.tb.Volume, miss: leaf, reads: map[walkKey]int{}}
	kept := &walkKept{refs: map[walkKey]int{}}
	var holes []WalkHole
	var holesMtx sync.Mutex
	conf := ww.conf(src, 8)
	conf.Keep = kept
	conf.Holes = func(h WalkHole) {
		holesMtx.Lock()
		holes = append(holes, h)
		holesMtx.Unlock()
	}
	w, err := NewWalk(ww.le, conf, walkPinOf(head, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Run(ctx); err != nil {
		t.Fatal(err)
	}
	st := w.Snapshot()
	t.Logf("walk %v read %d", WalkFields(st), len(src.reads))

	// No block is read twice, the missing root is the one hole, and every
	// block read is kept with its decoded refs.
	for k, n := range src.reads {
		if n != 1 {
			t.Fatalf("block %x read %d times, want once", k[:6], n)
		}
	}
	if st.GetMissing() != 1 || len(holes) != 1 || holes[0].Reason != HoleMissing || st.GetErrors()+st.GetUntyped()+st.GetUndecodable() != 0 {
		t.Fatalf("holes %v, want the one missing root", holes)
	}
	if st.GetVisited() != uint64(len(src.reads)) || st.GetReads() != uint64(len(src.reads)-1) {
		t.Fatalf("visited %d read %d, want %d and %d", st.GetVisited(), st.GetReads(), len(src.reads), len(src.reads)-1)
	}
	if len(kept.refs) != len(src.reads)-1 || st.GetKeptDecoded() != uint64(len(src.reads)-1) {
		t.Fatalf("kept %d, want the %d blocks read", len(kept.refs), len(src.reads)-1)
	}
	if st.GetOkraPages() < 3 || st.GetObjects() < objects {
		t.Fatalf("okra pages %d objects %d, want several pages and at least %d objects", st.GetOkraPages(), st.GetObjects(), objects)
	}
	if st.GetWorldDoneAt() == 0 || st.GetStage() != "done" || st.GetDoneAt() == 0 {
		t.Fatalf("stage %q world done at %d, want done", st.GetStage(), st.GetWorldDoneAt())
	}

	// The blocks the walk read alone open the World: every graph quad, so
	// every okra page of the graph tree, and every object's blob but the
	// missing one.
	read := &walkReadOnly{StoreOps: ww.tb.Volume, allow: func(ref *block.BlockRef) bool {
		k := keyOf(ref)
		return k != leaf && src.reads[k] != 0
	}}
	snap, err := world_block.OpenSnapshot(ctx, ww.le, read, head)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	tx, err := snap.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	got, err := tx.LookupGraphQuads(ctx, world.NewGraphQuad("", "", "", ""), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < quads {
		t.Fatalf("read %d graph quads from the walk's blocks, want %d", len(got), quads)
	}
	for i := range objects {
		objKey := "obj-" + strconv.Itoa(i)
		obj, found, err := tx.GetObject(ctx, objKey)
		if err != nil || !found {
			t.Fatalf("object %s: found=%t %v", objKey, found, err)
		}
		_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
			_, err := blob.FetchToBytes(ctx, bcs)
			return err
		})
		world.ReleaseObjectState(obj)
		if (err != nil) != (i == 0) {
			t.Fatalf("read object %s from the walk's blocks: %v", objKey, err)
		}
	}
}

// walkReference walks the pin uninterrupted and returns the blocks it read.
func walkReference(t *testing.T, ww *walkWorld, pin *WalkPin) map[walkKey]int {
	t.Helper()
	src := &walkHiddenRefs{store: ww.tb.Volume, reads: map[walkKey]int{}}
	w, err := NewWalk(ww.le, ww.conf(src, 8), pin)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if st := w.Snapshot(); st.GetMissing()+st.GetErrors()+st.GetUntyped()+st.GetUndecodable() != 0 {
		t.Fatalf("reference walk holes %v", st.GetFirstHoles())
	}
	return src.readSet()
}

// sameKeys fails the test unless got and want hold the same blocks.
func sameKeys(t *testing.T, what string, got, want map[walkKey]int) {
	t.Helper()
	for k := range want {
		if got[k] == 0 {
			t.Fatalf("%s: block %x reachable from the pin was not read", what, k[:6])
		}
	}
	for k := range got {
		if want[k] == 0 {
			t.Fatalf("%s: block %x outside the pin was read", what, k[:6])
		}
	}
}

// TestWalkResume pins a World head, writes to the World after the pin, then
// walks the pin cancelling the walk after every few reads as a restart would,
// resuming each time from the state file. The reads together are exactly the
// blocks reachable from the pin, as an uninterrupted walk of the pin reads
// both before and after the writes, and no block written after the pin is
// read. With the types saved, a resume reads few blocks again.
func TestWalkResume(t *testing.T) {
	ctx := t.Context()
	ww := newWalkWorld(t, 12, 200<<10, 600)
	head := ww.head()
	pin := walkPinOf(head, 7)

	// Walk the pin before the writes.
	before := walkReference(t, ww, pin)

	// Write after the pin: a new object and more quads.
	ww.addObject(t, "obj-after", 100<<10)
	for i := range 50 {
		q := world.NewGraphQuadWithKeys("obj-after", "<after-"+strconv.Itoa(i)+">", "obj-0", "")
		if err := ww.ws.SetGraphQuad(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	after := ww.head()
	afterRoot := ww.objectRoot(t, after, "obj-after")

	// The writes do not change what a walk of the pin reads.
	want := walkReference(t, ww, pin)
	sameKeys(t, "uninterrupted walk after the writes", want, before)
	if want[keyOf(after.GetRootRef())] != 0 || want[keyOf(afterRoot)] != 0 {
		t.Fatal("the walk of the pin read a block written after it")
	}

	// Resume with each block's type saved, then with every type taken from
	// the saved paths, as for a type this build cannot name.
	t.Run("saved types", func(t *testing.T) {
		got, reads := walkRestarts(t, ww, pin, false)
		sameKeys(t, "resumed walk", got, want)
		if reads > len(want)+len(want)/20 {
			t.Fatalf("read %d blocks %d times, want few reads again", len(want), reads)
		}
	})
	t.Run("parent types", func(t *testing.T) {
		got, _ := walkRestarts(t, ww, pin, true)
		sameKeys(t, "resumed walk", got, want)
	})
}

// walkRestarts walks the pin, stopping each run 60 reads past the saved
// nodes it reads again, and resumes from the state file until the walk ends.
// With noTypes the walk saves no block's type. It returns the blocks read and
// the reads.
func walkRestarts(t *testing.T, ww *walkWorld, pin *WalkPin, noTypes bool) (map[walkKey]int, int) {
	ctx := t.Context()
	statePath := filepath.Join(t.TempDir(), "state")
	src := &walkHiddenRefs{store: ww.tb.Volume, reads: map[walkKey]int{}}
	var runs int
	for done := false; !done; runs++ {
		if runs > 1000 {
			t.Fatal("the walk did not finish")
		}

		// Stop the run 60 reads past the saved nodes it reads again.
		rctx, cancel := context.WithCancel(ctx)
		stopAt := int64(60)
		if hs, herr := ReadWalkState(statePath); herr == nil {
			stopAt += int64(hs.GetNodes()) //nolint:gosec
		}
		var n atomic.Int64
		src.hook = func(_ context.Context, _ *block.BlockRef) error {
			if n.Add(1) == stopAt {
				cancel()
			}
			return nil
		}

		// Start the walk, or resume it with the saved concurrency.
		conf := ww.conf(src, 8)
		conf.StatePath = statePath
		var w *Walk
		var err error
		if runs == 0 {
			w, err = NewWalk(ww.le, conf, pin)
		} else {
			conf.Concurrency = 0
			w, err = ResumeWalk(ww.le, conf)
		}
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		if noTypes {
			w.reg = map[string]block.Ctor{}
		}
		err = w.Run(rctx)
		cancel()
		done = err == nil
		if !done && rctx.Err() == nil {
			t.Fatal(err)
		}
		if w.Snapshot().GetConcurrency() != 8 {
			t.Fatalf("resume ran %d reads at once, want the saved 8", w.Snapshot().GetConcurrency())
		}
	}

	// The state records the end.
	state, err := ReadWalkState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if state.GetPhase() != WalkPhase_WALK_PHASE_DONE || state.GetEntries() != 0 || state.GetNodes() != 0 || state.GetPin().GetSeqno() != pin.GetSeqno() {
		t.Fatalf("final state phase %v entries %d nodes %d seqno %d", state.GetPhase(), state.GetEntries(), state.GetNodes(), state.GetPin().GetSeqno())
	}
	if state.GetResumes() != uint32(runs-1) { //nolint:gosec
		t.Fatalf("resumes %d, want %d", state.GetResumes(), runs-1)
	}
	if _, err := ResumeWalk(ww.le, conf0(ww, src, statePath)); err == nil {
		t.Fatal("resumed a done walk")
	}
	got := src.readSet()
	var reads int
	for _, n := range got {
		reads += n
	}
	t.Logf("%d runs read %d blocks %d times; resumes %d", runs, len(got), reads, state.GetResumes())
	return got, reads
}

// conf0 returns a walk config reading src with its state at statePath.
func conf0(ww *walkWorld, src WalkSource, statePath string) WalkConfig {
	conf := ww.conf(src, 0)
	conf.StatePath = statePath
	return conf
}

// TestWalkResumeCrash saves a walk's position while its reads are blocked in
// flight, keeps a copy of that file as a crash would leave it, and resumes
// from the copy. The reads that returned before the save and the resumed
// reads together are exactly the blocks reachable from the pin.
func TestWalkResumeCrash(t *testing.T) {
	ctx := t.Context()
	ww := newWalkWorld(t, 12, 200<<10, 600)
	pin := walkPinOf(ww.head(), 3)
	want := walkReference(t, ww, pin)

	// Block every read after the first 60 until the walk ends.
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state")
	src := &walkHiddenRefs{store: ww.tb.Volume, reads: map[walkKey]int{}}
	var n atomic.Int64
	blocked := make(chan struct{}, 64)
	src.hook = func(ctx context.Context, _ *block.BlockRef) error {
		if n.Add(1) <= 60 {
			return nil
		}
		blocked <- struct{}{}
		<-ctx.Done()
		return context.Cause(ctx)
	}
	conf := ww.conf(src, 8)
	conf.StatePath = statePath
	conf.PersistEvery = 20 * time.Millisecond
	w, err := NewWalk(ww.le, conf, pin)
	if err != nil {
		t.Fatal(err)
	}
	saved := make(chan struct{}, 1)
	w.persisted = func() {
		select {
		case saved <- struct{}{}:
		default:
		}
	}

	// Once every worker is blocked, wait for a periodic save and copy it.
	rctx, cancel := context.WithCancel(ctx)
	errc := make(chan error, 1)
	go func() { errc <- w.Run(rctx) }()
	for range 8 {
		<-blocked
	}
	<-saved
	<-saved
	firstReads := src.readSet()
	crashPath := filepath.Join(dir, "crash")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(crashPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-errc; err == nil {
		t.Fatal("the blocked walk finished")
	}
	header, err := ReadWalkState(crashPath)
	if err != nil {
		t.Fatal(err)
	}
	if header.GetEntries() == 0 {
		t.Fatal("the save holds no frontier")
	}
	t.Logf("crash save: %d entries %d nodes %d bytes", header.GetEntries(), header.GetNodes(), len(data))

	// Resume from the copy.
	resumed := &walkHiddenRefs{store: ww.tb.Volume, reads: map[walkKey]int{}}
	conf = ww.conf(resumed, 8)
	conf.StatePath = crashPath
	w, err = ResumeWalk(ww.le, conf)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Run(ctx); err != nil {
		t.Fatal(err)
	}
	got := resumed.readSet()
	for k, c := range firstReads {
		got[k] += c
	}
	sameKeys(t, "walk resumed after a crash", got, want)
}

// TestWalkStateAtomic checks that a save replaces the state file whole: a
// save left unfinished beside it does not change what the file reads, and a
// save that cannot write keeps the last position.
func TestWalkStateAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	ref, err := block.BuildBlockRef([]byte("root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	head := &bucket.ObjectRef{RootRef: ref}
	write := func(seqno uint64) error {
		_, err := writeWalkState(path, &WalkState{Pin: walkPinOf(head, seqno), Entries: 1}, nil, []*WalkEntry{{Ref: ref}})
		return err
	}

	// A save writes the file and leaves nothing beside it.
	if err := write(1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + walkStateTmp); !os.IsNotExist(err) {
		t.Fatalf("a save left %s: %v", walkStateTmp, err)
	}

	// A crash mid-save leaves a partial file beside the path; the path still
	// reads the last position, and the next save replaces the partial file.
	if err := os.WriteFile(path+walkStateTmp, []byte{0xff, 0xff, 0x01}, 0o600); err != nil {
		t.Fatal(err)
	}
	if st, err := ReadWalkState(path); err != nil || st.GetPin().GetSeqno() != 1 {
		t.Fatalf("after a partial save: seqno %d %v", st.GetPin().GetSeqno(), err)
	}
	if err := write(2); err != nil {
		t.Fatal(err)
	}
	if st, err := ReadWalkState(path); err != nil || st.GetPin().GetSeqno() != 2 {
		t.Fatalf("after a save: seqno %d %v", st.GetPin().GetSeqno(), err)
	}

	// A save that cannot write its file keeps the last position.
	if err := os.Mkdir(path+walkStateTmp, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path+walkStateTmp, "x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := write(3); err == nil {
		t.Fatal("a save through a directory succeeded")
	}
	if st, err := ReadWalkState(path); err != nil || st.GetPin().GetSeqno() != 2 {
		t.Fatalf("after a failed save: seqno %d %v", st.GetPin().GetSeqno(), err)
	}

	// Discarding removes both.
	if err := os.RemoveAll(path + walkStateTmp); err != nil {
		t.Fatal(err)
	}
	if err := DiscardWalkState(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("discard left the state: %v", err)
	}
	if err := DiscardWalkState(path); err != nil {
		t.Fatal(err)
	}
}

// TestWalkStateSize measures a state file of a million frontier entries under
// a path of thousands of nodes, and bounds its size per entry.
func TestWalkStateSize(t *testing.T) {
	const entries, nodes = 1_000_000, 4096
	refOf := func(i int) *block.BlockRef {
		ref, err := block.BuildBlockRef(binaryKey(i), nil)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	ns := make([]*WalkNode, nodes)
	for i := range ns {
		ns[i] = &WalkNode{Ref: refOf(-i - 1), Parent: uint64(i / 2), Stage: 1}
	}
	es := make([]*WalkEntry, entries)
	for i := range es {
		es[i] = &WalkEntry{Ref: refOf(i), Parent: uint64(i%nodes + 1), Stage: 1, OkraValue: i%3 == 0, Type: &WalkType{Ctor: 2, Xfrm: 1}}
	}
	path := filepath.Join(t.TempDir(), "state")
	head := &bucket.ObjectRef{RootRef: refOf(-nodes - 1)}
	start := time.Now()
	size, err := writeWalkState(path, &WalkState{Pin: walkPinOf(head, 1), Nodes: nodes, Entries: entries, Ctors: []string{"*block.Blob", "*blob.Chunk"}, Xfrms: [][]byte{make([]byte, 60)}}, ns, es)
	if err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	per := float64(size) / entries
	t.Logf("state of %d entries and %d nodes: %d bytes, %.1f bytes an entry, written in %v", entries, nodes, size, per, took)
	if per > 64 {
		t.Fatalf("%.1f bytes an entry, want at most 64", per)
	}

	// The file reads back whole.
	f, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(f)) != size {
		t.Fatalf("read %d bytes, wrote %d", len(f), size)
	}
	st, err := ReadWalkState(path)
	if err != nil || st.GetEntries() != entries || st.GetNodes() != nodes {
		t.Fatalf("read back %d entries %d nodes: %v", st.GetEntries(), st.GetNodes(), err)
	}
}

// binaryKey returns a block's bytes numbered i.
func binaryKey(i int) []byte {
	h := sha256.Sum256([]byte(strconv.Itoa(i)))
	return h[:]
}

// walkReadOnly serves only the blocks allow admits.
type walkReadOnly struct {
	block.StoreOps
	allow func(ref *block.BlockRef) bool
}

// BeginReadOperation keeps reads in this store.
func (r *walkReadOnly) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return r, func() {}, nil
}

// GetBlock serves an admitted block.
func (r *walkReadOnly) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if !r.allow(ref) {
		return nil, false, nil
	}
	return r.StoreOps.GetBlock(ctx, ref)
}

// GetStoredBlock serves an admitted block.
func (r *walkReadOnly) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if !r.allow(ref) {
		return nil, nil
	}
	return r.StoreOps.GetStoredBlock(ctx, ref)
}

// walkSlowKeep records the blocks kept, and holds each keep until release
// closes, signalling entered as it starts.
type walkSlowKeep struct {
	entered chan struct{}
	release chan struct{}
	mtx     sync.Mutex
	kept    []walkKey
}

// StoreDecodedRefs waits for release, then records the block.
func (k *walkSlowKeep) StoreDecodedRefs(_ context.Context, ref *block.BlockRef, _ []byte, _ []*block.BlockRef) error {
	select {
	case k.entered <- struct{}{}:
	default:
	}
	<-k.release
	k.mtx.Lock()
	k.kept = append(k.kept, keyOf(ref))
	k.mtx.Unlock()
	return nil
}

// TestWalkKeepYields checks that a walk does not keep a block the World's
// packs hold, and defers a keep while every keep slot is busy rather than
// waiting for one.
func TestWalkKeepYields(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	refOf := func(s string) *block.BlockRef {
		r, err := block.BuildBlockRef([]byte(s), nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	held, slow, busy := refOf("held"), refOf("slow"), refOf("busy")
	keep := &walkSlowKeep{entered: make(chan struct{}, 1), release: make(chan struct{})}
	w, err := NewWalk(le, WalkConfig{
		Keep: keep,
		Locate: func(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
			out := make([]bool, len(refs))
			for i, r := range refs {
				out[i] = keyOf(r) == keyOf(held)
			}
			return out, nil
		},
		KeepSlots:   1,
		Concurrency: 1,
	}, walkPinOf(&bucket.ObjectRef{RootRef: held}, 1))
	if err != nil {
		t.Fatal(err)
	}

	// The held block is not kept.
	w.keepDecoded(ctx, walkItem{ref: held}, []byte("held"), nil)

	// One keep takes the only slot; another meanwhile is deferred at once.
	done := make(chan struct{})
	go func() {
		w.keepDecoded(ctx, walkItem{ref: slow}, []byte("slow"), nil)
		close(done)
	}()
	<-keep.entered
	w.keepDecoded(ctx, walkItem{ref: busy}, []byte("busy"), nil)

	// Once the slot frees, the next keep takes it.
	close(keep.release)
	<-done
	w.keepDecoded(ctx, walkItem{ref: busy}, []byte("busy"), nil)

	st := w.Snapshot()
	if st.GetKeepHeld() != 1 || st.GetKeepDeferred() != 1 || st.GetKeptDecoded() != 2 || st.GetKeepFailed() != 0 {
		t.Fatalf("held %d deferred %d kept %d failed %d, want 1 1 2 0", st.GetKeepHeld(), st.GetKeepDeferred(), st.GetKeptDecoded(), st.GetKeepFailed())
	}
	if len(keep.kept) != 2 || keep.kept[0] != keyOf(slow) || keep.kept[1] != keyOf(busy) {
		t.Fatalf("kept %d blocks, want slow then busy", len(keep.kept))
	}
}
