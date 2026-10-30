package block_gc

import (
	"bytes"
	"context"
	"encoding/binary"
	"maps"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Count records. A store whose counts record says complete keeps, for each
// block, the number of its holders: the owner edges from nodes that are not
// blocks, and the blocks that hold it. While the counts are complete:
//
//   - every block with a child list has a count record;
//   - a block's count is its owner edges from nodes that are not blocks plus
//     the holding blocks whose lists name it;
//   - a block holds the children in its list while its count is above zero,
//     and at zero until its children are released, which its record notes;
//   - a block whose count is zero has one free queue entry, which its record
//     names, and a block above zero has none.
//
// A count record without a child list is a hole: a block that a holder names
// and the store never held, or one a block pass swept while held.
//
// A write transaction sums its changes per block and writes only the blocks
// whose sum is not zero, when its body ends. A block that reaches zero
// releases its children within the transaction's cascade budget, or goes on
// the free queue still holding them. Counting a change the counts cannot
// explain, such as a holder leaving a block with no count, puts the counts in
// doubt: the counts record keeps the reason, and nothing is counted again
// until a rebuild. Doubt never fails the write.
//
// A store that tracked blocks before it kept counts builds them, as
// refgraph-count-build.go describes. While the build walks the graph, writes
// count only the holders whose records sort before its cursor: the build
// counts the rest when it reaches them.
const (
	// kindCount keys the count record of a block by the block.
	kindCount byte = 'k'
	// kindCounts is the one record of the counts' state.
	kindCounts byte = 's'
	// kindFree keys a free queue entry by a big-endian sequence. Its value is
	// the big-endian Unix nanoseconds it was queued at, then the block.
	kindFree byte = 'q'
	// kindFreeHead is the one record of the free queue's next sequence.
	kindFreeHead byte = 'h'
)

// The first byte of the counts record names the counts' phase.
const (
	// countsDoubt begins the record of counts in doubt, followed by the
	// reason.
	countsDoubt byte = 0
	// countsComplete is the record of complete counts.
	countsComplete byte = 1
	// countsWalking begins the record of a build that walks the child lists
	// and owner edges, followed by the key of the next record to count.
	countsWalking byte = 2
	// countsChecking begins the record of a build that checks the blocks
	// with a count and no child list, followed by the key of the next count
	// record to check.
	countsChecking byte = 3
)

// countCascadeBase is the number of child lists one write transaction may
// read to release children, beside countCascadePerList for each list it
// creates.
const countCascadeBase = 64

// countCascadePerList is the cascade budget each created child list adds.
const countCascadePerList = 4

var (
	// countsKey is the key of the counts record.
	countsKey = []byte{kindCounts}
	// freeHeadKey is the key of the free queue's next sequence.
	freeHeadKey = []byte{kindFreeHead}
)

// CountPhase is the phase of a store's counts.
type CountPhase int

const (
	// CountsAbsent is a store whose counts never started.
	CountsAbsent CountPhase = iota
	// CountsWalking is a build counting the child lists and owner edges.
	CountsWalking
	// CountsChecking is a build checking that no counted block is stored
	// without a child list.
	CountsChecking
	// CountsComplete is a store whose every write keeps the counts.
	CountsComplete
	// CountsDoubt is a store whose counts stopped; Doubt says why.
	CountsDoubt
)

// CountState reports the store's counts.
type CountState struct {
	// Phase is the counts' phase.
	Phase CountPhase
	// Doubt is the reason the counts stopped, in CountsDoubt.
	Doubt string
}

// Building reports whether the counts are being built.
func (s CountState) Building() bool {
	return s.Phase == CountsWalking || s.Phase == CountsChecking
}

// decodeCountState decodes the counts record.
func decodeCountState(value []byte, found bool) CountState {
	if !found || len(value) == 0 {
		return CountState{}
	}
	switch value[0] {
	case countsComplete:
		return CountState{Phase: CountsComplete}
	case countsWalking:
		return CountState{Phase: CountsWalking}
	case countsChecking:
		return CountState{Phase: CountsChecking}
	}
	return CountState{Phase: CountsDoubt, Doubt: string(value[1:])}
}

// countRecord is the stored count of a block.
type countRecord struct {
	// count is the number of holders.
	count uint64
	// seq is the free queue entry at zero.
	seq uint64
	// holds reports, at zero, that the children are not yet released.
	holds bool
}

// encode encodes a count above zero as its uvarint, and a zero count as a
// zero, the queue sequence and the holds byte.
func (r countRecord) encode() []byte {
	// A count above zero is the count alone.
	value := binary.AppendUvarint(nil, r.count)
	if r.count != 0 {
		return value
	}

	// A zero count adds its queue entry and whether it holds.
	value = binary.AppendUvarint(value, r.seq)
	if r.holds {
		return append(value, 1)
	}
	return append(value, 0)
}

// decodeCountRecord decodes a count record.
func decodeCountRecord(value []byte) (countRecord, error) {
	// Read the count.
	count, n := binary.Uvarint(value)
	if n <= 0 {
		return countRecord{}, errBadCountRecord
	}
	if count != 0 {
		return countRecord{count: count}, nil
	}

	// Read the queue entry and the holds byte of a zero count.
	seq, m := binary.Uvarint(value[n:])
	if m <= 0 || len(value) != n+m+1 {
		return countRecord{}, errBadCountRecord
	}
	return countRecord{seq: seq, holds: value[n+m] == 1}, nil
}

// errBadCountRecord reports a count record that does not decode.
var errBadCountRecord = errors.New("decode block count record")

// freeKey returns the key of free queue entry seq.
func freeKey(seq uint64) []byte {
	return binary.BigEndian.AppendUint64([]byte{kindFree}, seq)
}

// counter collects one write transaction's count changes, netted per block,
// and writes them when the transaction's body ends.
type counter struct {
	// on reports that the counts are complete or being built, and changes
	// are kept.
	on bool
	// phase is the first byte of the counts record while on.
	phase byte
	// cursor is the key of the next record a build counts or checks. While
	// the build walks, only holders whose records sort before it are
	// counted.
	cursor []byte
	// doubt is the reason the counts went into doubt in this transaction.
	doubt string
	// delta is the net change per block.
	delta map[string]int64
	// created holds the blocks whose child lists the transaction created.
	created map[string]struct{}
	// dropped holds the blocks whose child lists the transaction deleted.
	dropped map[string]struct{}
}

// openCounter returns the counter of a write transaction, which counts only
// when the counts record says complete or building.
func openCounter(ctx context.Context, tx kvtx.TxOps) (*counter, error) {
	// Read the counts record; without one nothing is counted.
	value, found, err := tx.Get(ctx, countsKey)
	if err != nil || !found || len(value) == 0 {
		return &counter{}, err
	}

	// Count while complete or building.
	c := &counter{phase: value[0], cursor: bytes.Clone(value[1:])}
	c.on = c.phase == countsComplete || c.phase == countsWalking || c.phase == countsChecking
	return c, nil
}

// counted reports whether the holder whose record is the graphKey of kind and
// nodes is counted: always, unless a build walks and has not reached it.
func (c *counter) counted(kind byte, nodes ...string) bool {
	return c.phase != countsWalking || bytes.Compare(graphKey(kind, nodes...), c.cursor) < 0
}

// listCounted reports whether block's child list is counted.
func (c *counter) listCounted(block string) bool {
	return c.counted(kindChildren, block)
}

// add changes the net count of block by n.
func (c *counter) add(block string, n int64) {
	if c.delta == nil {
		c.delta = make(map[string]int64)
	}
	c.delta[block] += n
}

// distrust puts the counts in doubt for reason and stops counting.
func (c *counter) distrust(reason string) {
	if c.on {
		c.on, c.doubt = false, reason
	}
}

// ownerAdded counts a new owner edge from a node that is not a block.
func (c *counter) ownerAdded(subject, object string) {
	if c.on && isBlockNode(object) && c.counted(kindIncoming, object, subject) {
		c.add(object, 1)
	}
}

// ownerRemoved counts a removed owner edge from a node that is not a block.
func (c *counter) ownerRemoved(subject, object string) {
	if c.on && isBlockNode(object) && c.counted(kindIncoming, object, subject) {
		c.add(object, -1)
	}
}

// listCreated counts the children of a block's new child list, which the
// block holds from its first write.
func (c *counter) listCreated(block string, children []string) {
	if !c.on || !c.listCounted(block) {
		return
	}
	if c.created == nil {
		c.created = make(map[string]struct{})
	}
	c.created[block] = struct{}{}
	for _, child := range children {
		c.add(child, 1)
	}
}

// childrenChanged counts children added to, or removed from, the existing
// list of parent by n each, when parent holds its children.
func (c *counter) childrenChanged(ctx context.Context, tx kvtx.TxOps, parent string, children []string, n int64) error {
	// Count only the changes of a counted parent that holds.
	if !c.on || len(children) == 0 || !c.listCounted(parent) {
		return nil
	}
	holds, err := c.holds(ctx, tx, parent)
	if err != nil || !holds {
		return err
	}
	for _, child := range children {
		c.add(child, n)
	}
	return nil
}

// listDropped counts the deletion of a block's child list: the block releases
// the children it held, if its list was counted, and its count record goes
// when its count is zero.
func (c *counter) listDropped(ctx context.Context, tx kvtx.TxOps, block string, children []string) error {
	if err := c.childrenChanged(ctx, tx, block, children, -1); err != nil || !c.on {
		return err
	}
	if c.dropped == nil {
		c.dropped = make(map[string]struct{})
	}
	c.dropped[block] = struct{}{}
	return nil
}

// holds reports whether block held its children when the transaction began,
// or created its list in the transaction. Changes to a list are counted
// against that state: the flush that moves the block's count across zero then
// reads the list as the transaction left it.
func (c *counter) holds(ctx context.Context, tx kvtx.TxOps, block string) (bool, error) {
	// A list the transaction created is held.
	if _, ok := c.created[block]; ok {
		return true, nil
	}

	// Read the stored record. A listed block without one cannot be counted.
	rec, found, err := readCountTx(ctx, tx, block)
	if err != nil {
		return false, err
	}
	if !found {
		c.distrust("child list of " + FormatIRI(block) + " has no count")
		return false, nil
	}
	return rec.count != 0 || rec.holds, nil
}

// readCountTx reads a block's count record.
func readCountTx(ctx context.Context, tx kvtx.TxOps, block string) (countRecord, bool, error) {
	value, found, err := tx.Get(ctx, graphKey(kindCount, block))
	if err != nil || !found {
		return countRecord{}, found, err
	}
	rec, err := decodeCountRecord(value)
	return rec, true, err
}

// prefetchKeys lists the count records a batch of edges may change.
func (c *counter) prefetchKeys(adds, removes []RefEdge) [][]byte {
	if !c.on {
		return nil
	}
	keys := make([][]byte, 0, len(adds)+len(removes))
	for _, edges := range [][]RefEdge{adds, removes} {
		for _, edge := range edges {
			if parent, _, ok := childEdge(edge); ok {
				keys = append(keys, graphKey(kindCount, parent))
			}
			if isBlockNode(edge.Object) {
				keys = append(keys, graphKey(kindCount, edge.Object))
			}
		}
	}
	return keys
}

// flush writes the transaction's netted counts. A block that reaches zero
// joins the free queue and, within the cascade budget, releases its children,
// whose changes the next round writes; past the budget it keeps holding them.
// A block that leaves zero leaves the queue, and holds its children again if
// it had released them. A block whose list the build has not counted neither
// releases nor holds: it is queued without holding, and holds once the build
// counts its list.
func (c *counter) flush(ctx context.Context, tx kvtx.TxOps) error {
	// Record a doubt, or stop when not counting.
	if !c.on {
		if c.doubt == "" {
			return nil
		}
		return tx.Set(ctx, countsKey, append([]byte{countsDoubt}, c.doubt...))
	}

	// Visit every block the transaction changed, created or dropped.
	work := c.delta
	if work == nil {
		work = make(map[string]int64)
	}
	for block := range c.created {
		work[block] += 0
	}
	for block := range c.dropped {
		work[block] += 0
	}
	f := &countFlush{
		counter: c,
		budget:  countCascadeBase + countCascadePerList*len(c.created),
		now:     time.Now(),
	}
	for len(work) != 0 {
		next := make(map[string]int64)
		for _, block := range slices.Sorted(maps.Keys(work)) {
			if err := f.apply(ctx, tx, block, work[block], next); err != nil {
				return err
			}
			if !c.on {
				return c.flush(ctx, tx)
			}
		}
		work = next
		c.created, c.dropped = nil, nil
	}
	if f.headDirty {
		return tx.Set(ctx, freeHeadKey, binary.AppendUvarint(nil, f.head))
	}
	return nil
}

// countFlush is the state of one flush.
type countFlush struct {
	// counter is the transaction's counter.
	counter *counter
	// budget is the number of child lists left to read for releases.
	budget int
	// now is the time free queue entries record.
	now time.Time
	// head is the next free queue sequence, once read.
	head uint64
	// headRead reports that head was read.
	headRead bool
	// headDirty reports that head changed.
	headDirty bool
}

// apply writes one block's net change d and adds the changes it causes to its
// children to next.
func (f *countFlush) apply(ctx context.Context, tx kvtx.TxOps, block string, d int64, next map[string]int64) error {
	// Read the stored record.
	c := f.counter
	_, created := c.created[block]
	_, dropped := c.dropped[block]
	rec, found, err := readCountTx(ctx, tx, block)
	if err != nil {
		return err
	}

	// Compute the new count. A change the counts cannot explain distrusts
	// them.
	if !found && d < 0 {
		c.distrust("holder left " + FormatIRI(block) + ", which has no count")
		return nil
	}
	sum := int64(rec.count) + d //nolint:gosec // counts stay far below 2^63.
	if sum < 0 {
		c.distrust("count of " + FormatIRI(block) + " fell below zero")
		return nil
	}
	count := uint64(sum)
	key := graphKey(kindCount, block)
	wasZero := found && rec.count == 0

	// A dropped list leaves a hole while holders remain, and nothing at zero.
	if dropped {
		if wasZero {
			if err := tx.Delete(ctx, freeKey(rec.seq)); err != nil {
				return err
			}
		}
		if count == 0 {
			if !found {
				return nil
			}
			return tx.Delete(ctx, key)
		}
		return tx.Set(ctx, key, countRecord{count: count}.encode())
	}

	// A block above zero leaves the queue and holds its children again.
	if count != 0 {
		if wasZero {
			if err := tx.Delete(ctx, freeKey(rec.seq)); err != nil {
				return err
			}
			if !rec.holds && !created && c.listCounted(block) {
				if err := f.cascade(ctx, tx, block, 1, next); err != nil {
					return err
				}
			}
		}
		if found && rec.count == count {
			return nil
		}
		return tx.Set(ctx, key, countRecord{count: count}.encode())
	}

	// A block still at zero keeps its entry; a list created for it is held.
	if wasZero {
		if !created || rec.holds {
			return nil
		}
		rec.holds = true
		return tx.Set(ctx, key, rec.encode())
	}

	// A block reaching zero joins the queue. One that had holders releases
	// its children within the budget; a new block holds the list it created.
	seq, err := f.enqueue(ctx, tx, block)
	if err != nil {
		return err
	}
	rec = countRecord{seq: seq, holds: created}
	if found && c.listCounted(block) {
		rec.holds = true
		if f.budget > 0 {
			f.budget--
			if err := f.cascade(ctx, tx, block, -1, next); err != nil {
				return err
			}
			rec.holds = false
		}
	}
	return tx.Set(ctx, key, rec.encode())
}

// cascade adds n to the next change of each child in block's list.
func (f *countFlush) cascade(ctx context.Context, tx kvtx.TxOps, block string, n int64, next map[string]int64) error {
	children, _, err := readChildrenTx(ctx, tx, block)
	for _, child := range children {
		next[child] += n
	}
	return err
}

// enqueue appends block to the free queue and returns its sequence.
func (f *countFlush) enqueue(ctx context.Context, tx kvtx.TxOps, block string) (uint64, error) {
	// Read the next sequence once per flush.
	if !f.headRead {
		value, found, err := tx.Get(ctx, freeHeadKey)
		if err != nil {
			return 0, err
		}
		if found {
			head, n := binary.Uvarint(value)
			if n <= 0 {
				return 0, errors.New("decode free queue head")
			}
			f.head = head
		}
		f.headRead = true
	}

	// Write the entry under the next sequence.
	seq := f.head
	f.head++
	f.headDirty = true
	value := binary.BigEndian.AppendUint64(nil, uint64(f.now.UnixNano())) //nolint:gosec // the clock is after 1970.
	value = appendNode(value, block)
	return seq, tx.Set(ctx, freeKey(seq), value)
}
