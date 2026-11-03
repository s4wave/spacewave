package block_gc

import (
	"bytes"
	"context"
	"encoding/binary"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// The sharing walk. A graph whose blocks were shared before it kept shared
// counts builds them by counting every holder of every block once, in slices,
// while writes continue. A block's holders are:
//
//   - each child list that lists it;
//   - each owner edge to it from a node other than the unreferenced queue;
//   - each block an owner edge's object holds without naming it, which a
//     HolderSource reports, such as the old root of a named deadlist record.
//
// Counting one holder of a block is a sighting. The first sighting leaves a
// seen record and each later one adds one to the block's shared count, so the
// count ends at the block's holders minus one.
//
// The walk clears the shared counts, the pending releases and the seen
// records, sights the children of each child list, then the block of each
// owner edge and what its object holds, in key order. The cursor is the last
// walked key followed by a zero byte, so a record is before the cursor exactly
// when the phase walked it. While the walk sights, a write sights each child
// it newly adds to a list, and each block it newly gives an owner edge, when
// that record is before the cursor: a list once the children phase passed it
// or ended, an owner edge once the owners phase passed it. The walk sights a record after the cursor
// when it reaches it, and a write while clearing sights nothing. Removing an
// edge sights nothing, so a holder sighted and then removed over-counts,
// which keeps a block the rule would free and never frees one it keeps.
//
// After the owner edges the counts are complete, and in the same commit the
// writers return to keeping them incrementally, as Counting reports. The walk
// then deletes the seen records, which nothing reads any more.
//
// The state record is the one source of the phase and cursor: each write reads
// it in its own transaction, so no write applies a rule from a stale copy.
//
// A first sighting also checks that the block's references are known: a block
// without a child list may receive a later incarnation whose first holder
// the walk already counted, so the counts stay doubtful until a fresh walk.

// SharingPhase is the phase of a graph's sharing walk.
type SharingPhase byte

const (
	// SharingComplete has complete shared counts: a graph that never walked
	// or whose walk ended. It has no state record unless in doubt.
	SharingComplete SharingPhase = 0
	// SharingOff keeps no shared counts: they are stale until a new walk.
	SharingOff SharingPhase = 'o'
	// SharingClearingHolds deletes the stale shared counts.
	SharingClearingHolds SharingPhase = 'h'
	// SharingClearingReleases deletes the pending releases, which the
	// stale counts included.
	SharingClearingReleases SharingPhase = 'l'
	// SharingClearingSeen deletes the seen records of an earlier walk.
	SharingClearingSeen SharingPhase = 'z'
	// SharingChildren sights the children of each child list.
	SharingChildren SharingPhase = 'c'
	// SharingOwners sights the block of each owner edge.
	SharingOwners SharingPhase = 'i'
	// SharingForgetting has complete counts and deletes the seen records.
	SharingForgetting SharingPhase = 's'
)

// Counting reports whether the phase keeps incremental shared counts.
// Fate decisions and receipt certification also require a state without Doubt.
func (p SharingPhase) Counting() bool {
	return p == SharingComplete || p == SharingForgetting
}

// Walking reports whether the walk is building the counts.
func (p SharingPhase) Walking() bool {
	switch p {
	case SharingClearingHolds, SharingClearingReleases, SharingClearingSeen, SharingChildren, SharingOwners:
		return true
	}
	return false
}

// Running reports whether the walk has slices left to run: it is building
// the counts, or forgetting its seen records after completing them.
func (p SharingPhase) Running() bool {
	return p.Walking() || p == SharingForgetting
}

// SharingState is the state of a graph's sharing walk.
type SharingState struct {
	// Phase is the walk's phase.
	Phase SharingPhase
	// Doubt reports an unknown child list, an erased sighted count prefix,
	// or a registered block root the walk cannot count. Fate decisions and
	// certified receipts wait for a fresh walk to rebuild known counts.
	Doubt bool
	// cursor is the key after the last record the phase walked.
	cursor []byte
}

// HolderSource answers what the sharing walk asks of the block store, in the
// transaction the graph writes in.
type HolderSource interface {
	// Implied visits the holders object keeps without graph edges, given its
	// owner edge. Repeated visits represent distinct holders of the same block.
	Implied(ctx context.Context, owner, object string, visit func(string) error) error
}

// ErrSharingWalk reports a change to the shared counts while they are not
// complete: the sharing walk is building them, or the graph keeps none.
var ErrSharingWalk = errors.New("shared counts are not complete")

// sharingDoubt is the flag of a state record in doubt.
const sharingDoubt byte = 1

// maxSharingSlice bounds the records one slice of the walk reads.
const maxSharingSlice = 1024

// sharingKey is the key of the state record.
var sharingKey = []byte{kindSharing}

// sharingGenerationKey survives completed walks. Rebuilding or erasing an
// incarnation's count metadata invalidates receipts from the preceding basis.
var sharingGenerationKey = []byte{kindSharing, 0}

// GetSharingGeneration returns the persisted receipt funding basis. An actual
// rebuild or erasure of an incarnation's count metadata advances it. Completion
// and requesting an already-running walk alone leave it unchanged.
func (g *RefGraph) GetSharingGeneration(ctx context.Context) (uint64, error) {
	var generation uint64
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		generation, err = readSharingGenerationTx(ctx, tx)
		return err
	})
	return generation, err
}

// readSharingGenerationTx reads the one counting generation in tx.
func readSharingGenerationTx(ctx context.Context, tx kvtx.TxOps) (uint64, error) {
	// Missing generation belongs to the initial complete count basis.
	data, found, err := tx.Get(ctx, sharingGenerationKey)
	if err != nil || !found {
		return 0, err
	}
	n, size := binary.Uvarint(data)
	if size <= 0 || size != len(data) {
		return 0, errors.New("sharing generation is malformed")
	}
	return n, nil
}

// readSharingTx reads the state of the sharing walk.
func readSharingTx(ctx context.Context, tx kvtx.TxOps) (SharingState, error) {
	value, found, err := tx.Get(ctx, sharingKey)
	if err != nil || !found {
		return SharingState{}, err
	}
	if len(value) < 2 {
		return SharingState{}, errors.New("sharing walk state is malformed")
	}
	return SharingState{
		Phase:  SharingPhase(value[0]),
		Doubt:  value[1]&sharingDoubt != 0,
		cursor: bytes.Clone(value[2:]),
	}, nil
}

// writeSharingTx writes the state of the sharing walk, deleting the record of
// complete counts that are not in doubt.
func writeSharingTx(ctx context.Context, tx kvtx.TxOps, s SharingState) error {
	if s.Phase == SharingComplete && !s.Doubt {
		return tx.Delete(ctx, sharingKey)
	}
	var flags byte
	if s.Doubt {
		flags |= sharingDoubt
	}
	return tx.Set(ctx, sharingKey, append([]byte{byte(s.Phase), flags}, s.cursor...))
}

// SetHolderSource supplies count-only holders not represented by graph edges.
// Without one, owner edges imply no additional holders. Missing child lists
// always put the walk in doubt, independently of implied holders.
func (g *RefGraph) SetHolderSource(src HolderSource) {
	g.holders = src
}

// GetSharing returns the state of the sharing walk.
func (g *RefGraph) GetSharing(ctx context.Context) (SharingState, error) {
	var s SharingState
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		s, err = readSharingTx(ctx, tx)
		return err
	})
	return s, err
}

// BeginSharing starts a new sharing walk, or completes the counts at once when
// the graph tracks no block, so every count starts at its block's first write.
// A walk already running, or forgetting after complete counts, keeps its
// place.
func (g *RefGraph) BeginSharing(ctx context.Context) (SharingState, error) {
	return g.beginSharing(ctx, SharingPhase.Running)
}

// RecountSharing starts a new sharing walk like BeginSharing, after a writer
// lost holders it would have counted: complete counts may be short, so only a
// walk still building the counts keeps its place, since it sights every
// holder the graph holds.
func (g *RefGraph) RecountSharing(ctx context.Context) (SharingState, error) {
	return g.beginSharing(ctx, SharingPhase.Walking)
}

// beginSharing starts a new sharing walk unless keep reports that the current
// phase keeps its place.
func (g *RefGraph) beginSharing(ctx context.Context, keep func(SharingPhase) bool) (SharingState, error) {
	var s SharingState
	err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		// Keep a walk that keep accepts.
		var err error
		if s, err = readSharingTx(ctx, tx); err != nil || keep(s.Phase) {
			return err
		}

		s, err = g.restartSharingTx(ctx, tx)
		return err
	})
	return s, err
}

// restartSharingTx starts a rebuild in the caller's physical transaction.
// A caller preserves an active walk before calling it.
func (g *RefGraph) restartSharingTx(ctx context.Context, tx kvtx.Tx) (SharingState, error) {
	// Empty graphs need no cursor; tracked graphs rebuild from their first hold.
	tracked, err := tracksBlocksTx(ctx, tx)
	if err != nil {
		return SharingState{}, err
	}
	s := SharingState{}
	if tracked {
		if err := g.invalidateSharingBasisTx(ctx, tx); err != nil {
			return s, err
		}
		s = SharingState{Phase: SharingClearingHolds, cursor: []byte{kindHold}}
	}
	return s, writeSharingTx(ctx, tx, s)
}

// StopSharing stops keeping shared counts. They are stale from now on, and
// only a new walk, begun by BeginSharing, makes them complete again.
func (g *RefGraph) StopSharing(ctx context.Context) error {
	return g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		return writeSharingTx(ctx, tx, SharingState{Phase: SharingOff})
	})
}

// TracksBlocks reports whether the graph tracks a block: whether it holds a
// child list, or an owner edge or a registered root of a block.
func (g *RefGraph) TracksBlocks(ctx context.Context) (bool, error) {
	var tracked bool
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		tracked, err = tracksBlocksTx(ctx, tx)
		return err
	})
	return tracked, err
}

// tracksBlocksTx reports whether tx holds a child list, or an owner edge or a
// registered root of a block. A block node begins with its hash type, which
// sorts after a text node's zero byte.
func tracksBlocksTx(ctx context.Context, tx kvtx.Tx) (bool, error) {
	for _, prefix := range [][]byte{{kindChildren}, {kindIncoming, textNode + 1}, {kindRoot, textNode + 1}} {
		if tracked, err := hasRecordTx(ctx, tx, prefix); err != nil || tracked {
			return tracked, err
		}
	}
	return false, nil
}

// hasRecordTx reports whether a record of prefix's kind sorts at or after
// prefix.
func hasRecordTx(ctx context.Context, tx kvtx.Tx, prefix []byte) (bool, error) {
	it := tx.Iterate(ctx, prefix[:1], false, false)
	defer it.Close()
	if err := it.Seek(prefix); err != nil {
		return false, err
	}
	return it.Valid(), it.Err()
}

// WalkSharing runs one slice of the sharing walk in one write transaction. It
// walks up to maxSharingSlice records, stopping early once yield reports a
// spent budget or a waiting writer, after the first. It returns the state
// after the slice; a phase that is neither Walking nor SharingForgetting has
// nothing to walk.
func (g *RefGraph) WalkSharing(ctx context.Context, yield func() bool) (SharingState, error) {
	var s SharingState
	err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		// Read the state; only a walk or a forgetting has a slice to run.
		var err error
		if s, err = readSharingTx(ctx, tx); err != nil {
			return err
		}
		if !s.Phase.Running() {
			return nil
		}

		// Read the slice's records, which reads the first whatever yield says.
		kind := sharingKind(s.Phase)
		records, done, err := readSliceTx(ctx, tx, kind, s.cursor, yield)
		if err != nil {
			return err
		}

		// Walk them.
		w := sharingTx{tx: tx, holders: g.holders, state: &s}
		for i, r := range records {
			if i != 0 && yield() {
				done = false
				break
			}
			if err := w.walk(ctx, s.Phase, r); err != nil {
				return err
			}
			s.cursor = append(s.cursor[:0], r.key...)
			s.cursor = append(s.cursor, 0)
		}

		// Move to the next phase after the last record. A registered root
		// naming a block holds it, but the walk sights only child lists and
		// owner edges, so such a root puts the counts in doubt.
		if done {
			if s.Phase == SharingOwners && !s.Doubt {
				if s.Doubt, err = hasRecordTx(ctx, tx, []byte{kindRoot, textNode + 1}); err != nil {
					return err
				}
			}
			s.Phase, s.cursor = nextSharingPhase(s.Phase)
		}
		return writeSharingTx(ctx, tx, s)
	})
	return s, err
}

// sharingKind returns the record kind a phase walks.
func sharingKind(p SharingPhase) byte {
	switch p {
	case SharingClearingHolds:
		return kindHold
	case SharingClearingReleases:
		return kindRelease
	case SharingChildren:
		return kindChildren
	case SharingOwners:
		return kindIncoming
	}
	return kindSeen
}

// nextSharingPhase returns the phase after p and its first cursor.
func nextSharingPhase(p SharingPhase) (SharingPhase, []byte) {
	switch p {
	case SharingClearingHolds:
		return SharingClearingReleases, []byte{kindRelease}
	case SharingClearingReleases:
		return SharingClearingSeen, []byte{kindSeen}
	case SharingClearingSeen:
		return SharingChildren, []byte{kindChildren}
	case SharingChildren:
		return SharingOwners, []byte{kindIncoming, textNode + 1}
	case SharingOwners:
		return SharingForgetting, []byte{kindSeen}
	}
	return SharingComplete, nil
}

// sliceRecord is one record a slice of the walk read.
type sliceRecord struct {
	key, value []byte
}

// readSliceTx reads the records of kind from cursor, up to maxSharingSlice and
// until yield, after the first. It reports whether it read the last.
func readSliceTx(ctx context.Context, tx kvtx.Tx, kind byte, cursor []byte, yield func() bool) ([]sliceRecord, bool, error) {
	// Seek the cursor in the kind's records.
	it := tx.Iterate(ctx, []byte{kind}, true, false)
	defer it.Close()
	if err := it.Seek(cursor); err != nil {
		return nil, false, err
	}

	// Read up to the slice's bound, stopping early when yield asks.
	var records []sliceRecord
	for ; it.Valid(); it.Next() {
		if len(records) == maxSharingSlice || (len(records) != 0 && yield()) {
			return records, false, it.Err()
		}
		value, err := it.Value()
		if err != nil {
			return nil, false, err
		}
		records = append(records, sliceRecord{key: bytes.Clone(it.Key()), value: bytes.Clone(value)})
	}
	return records, true, it.Err()
}

// sharingTx sights blocks in one write transaction.
type sharingTx struct {
	// tx is the transaction.
	tx kvtx.Tx
	// holders answers questions about blocks, may be nil.
	holders HolderSource
	// state is the walk's state, whose doubt a sighting may set.
	state *SharingState
}

// walk handles one record the phase walks.
func (w *sharingTx) walk(ctx context.Context, phase SharingPhase, r sliceRecord) error {
	switch phase {
	case SharingChildren:
		// Sight each child of the list.
		children, err := decodeChildren(r.value)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := w.sight(ctx, child); err != nil {
				return err
			}
		}
		return nil
	case SharingOwners:
		// Sight the block of an owner edge and what its object holds.
		object, rest, err := readNode(r.key[1:])
		if err != nil {
			return err
		}
		subject, _, err := readNode(rest)
		if err != nil {
			return err
		}
		return w.sightOwned(ctx, subject, object)
	}

	// Clear a shared count or a seen record.
	return w.tx.Delete(ctx, r.key)
}

// sightOwned sights the block of an owner edge from subject, unless subject is
// the unreferenced queue, and the block the object holds without naming it.
func (w *sharingTx) sightOwned(ctx context.Context, subject, object string) error {
	// Sight the owned block.
	if subject == NodeUnreferenced || !isBlockNode(object) {
		return nil
	}
	if err := w.sight(ctx, object); err != nil {
		return err
	}

	// Sight the block the object implies.
	if w.holders == nil {
		return nil
	}
	return w.holders.Implied(ctx, subject, object, func(node string) error {
		return w.sight(ctx, node)
	})
}

// sight counts one holder of block: the first leaves a seen record and checks
// that the block's references are known, and each later one adds one to the
// block's shared count.
func (w *sharingTx) sight(ctx context.Context, block string) error {
	// Count a later sighting.
	seen := graphKey(kindSeen, block)
	found, err := w.tx.Exists(ctx, seen)
	if err != nil {
		return err
	}
	if found {
		n, err := readHoldsTx(ctx, w.tx, block)
		if err != nil {
			return err
		}
		return w.tx.Set(ctx, graphKey(kindHold, block), binary.AppendUvarint(nil, n+1))
	}

	// An absent child list also leaves the first implicit holder unknown.
	// A later fresh write must not certify a second first holder at this address.
	if !w.state.Doubt {
		tracked, err := w.tx.Exists(ctx, graphKey(kindChildren, block))
		if err != nil {
			return err
		}
		w.state.Doubt = !tracked
	}
	return w.tx.Set(ctx, seen, nil)
}

// sightWritesTx sights the holders a write adds before the walk's cursor: the
// children added to each list in lists, and each owner edge in owned. It does
// nothing unless the walk is sighting.
func (g *RefGraph) sightWritesTx(ctx context.Context, tx kvtx.Tx, lists map[string][]string, owned []RefEdge) error {
	// Read the state; only the sighting phases have a cursor writes respect.
	s, err := readSharingTx(ctx, tx)
	if err != nil {
		return err
	}
	if s.Phase.Counting() && s.Doubt && (len(lists) != 0 || len(owned) != 0) {
		_, err = g.restartSharingTx(ctx, tx)
		return err
	}
	if s.Phase != SharingChildren && s.Phase != SharingOwners {
		return nil
	}
	w := sharingTx{tx: tx, holders: g.holders, state: &s}
	doubt := s.Doubt

	// Sight the children added to lists the cursor passed.
	for parent, added := range lists {
		if bytes.Compare(graphKey(kindChildren, parent), s.cursor) >= 0 && s.Phase == SharingChildren {
			continue
		}
		for _, child := range added {
			if err := w.sight(ctx, child); err != nil {
				return err
			}
		}
	}

	// Sight the owned blocks whose edges the cursor passed.
	if s.Phase == SharingOwners {
		for _, edge := range owned {
			if bytes.Compare(graphKey(kindIncoming, edge.Object, edge.Subject), s.cursor) >= 0 {
				continue
			}
			if err := w.sightOwned(ctx, edge.Subject, edge.Object); err != nil {
				return err
			}
		}
	}

	// Keep a doubt a sighting found.
	if s.Doubt == doubt {
		return nil
	}
	return writeSharingTx(ctx, tx, s)
}
