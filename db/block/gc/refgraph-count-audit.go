package block_gc

import (
	"bytes"
	"context"
	"encoding/binary"
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// countAuditReport bounds the mismatches an audit reports.
const countAuditReport = 16

// ErrCountAuditFull reports an audit share whose blocks with holders passed
// the audit's bound; a smaller share fits.
var ErrCountAuditFull = errors.New("count audit share passed its block bound")

// CountAudit is the result of auditing one share of the counts.
type CountAudit struct {
	// State is the counts' state in the audited snapshot. Only complete and
	// building counts are audited.
	State CountState
	// Blocks is the number of count records the share compared.
	Blocks int
	// Mismatches holds up to the first 16 mismatches.
	Mismatches []CountMismatch
	// MismatchCount is the number of mismatches found.
	MismatchCount int
}

// AuditCounts checks the counts of one share of the blocks, share of shares,
// against the graph in one read snapshot, as the count invariants in
// refgraph-counts.go state. It sums each block's holders from the owner edges
// and the child lists of holding blocks, compares the sums with the count
// records, and checks the free queue against the zero counts. While a build
// walks, it sums only the records before the build's cursor. It reports and
// never writes. It returns ErrCountAuditFull when more than maxBlocks blocks of
// the share have holders, which bounds its memory.
//
// Each call reads every child list and owner edge of a block, and point-reads
// the count record of each list's block and each zero count's queue entry.
func (g *RefGraph) AuditCounts(ctx context.Context, share, shares uint32, maxBlocks int) (CountAudit, error) {
	var audit CountAudit
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		// Audit only complete or building counts.
		audit = CountAudit{}
		value, found, err := tx.Get(ctx, countsKey)
		if err != nil {
			return err
		}
		audit.State = decodeCountState(value, found)
		if audit.State.Phase != CountsComplete && !audit.State.Building() {
			return nil
		}
		a := &countAuditor{
			CountAudit: &audit,
			tx:         tx,
			share:      share,
			shares:     shares,
			maxBlocks:  maxBlocks,
			want:       make(map[string]uint64),
		}
		if audit.State.Phase == CountsWalking {
			a.cursor = value[1:]
		}

		// Sum the holders, then compare the count records and the queue.
		if err := a.sumLists(ctx); err != nil {
			return err
		}
		if err := a.sumOwners(ctx); err != nil {
			return err
		}
		if err := a.compareCounts(ctx); err != nil {
			return err
		}
		if err := a.compareQueue(ctx); err != nil {
			return err
		}

		// Report holders left without a count record.
		for block, n := range a.want {
			a.report(block, strconv.FormatUint(n, 10)+" holders and no count")
		}
		return nil
	})
	return audit, err
}

// DistrustCounts puts complete or building counts in doubt for reason, as an
// audit mismatch does. Counts already in doubt keep their first reason.
func (g *RefGraph) DistrustCounts(ctx context.Context, reason string) error {
	return g.update(ctx, func(_ context.Context, _ kvtx.Tx, c *counter) error {
		c.distrust(reason)
		return nil
	})
}

// countAuditor is the state of one audit.
type countAuditor struct {
	*CountAudit
	// tx is the audited snapshot.
	tx kvtx.Tx
	// share and shares select the audited blocks.
	share, shares uint32
	// maxBlocks bounds the blocks of want.
	maxBlocks int
	// cursor is the walking build's cursor, or nil.
	cursor []byte
	// want is the holders summed per block of the share.
	want map[string]uint64
}

// inShare reports whether block is in the audited share, by an FNV-1a hash of
// its IRI.
func (a *countAuditor) inShare(block string) bool {
	h := uint32(2166136261)
	for i := 0; i < len(block); i++ {
		h = (h ^ uint32(block[i])) * 16777619
	}
	return h%a.shares == a.share
}

// counted reports whether the record at key is counted: always, unless a
// build walks and has not reached it.
func (a *countAuditor) counted(key []byte) bool {
	return a.cursor == nil || bytes.Compare(key, a.cursor) < 0
}

// hold adds a holder of block when block is in the share.
func (a *countAuditor) hold(block string) error {
	if !a.inShare(block) {
		return nil
	}
	a.want[block]++
	if len(a.want) > a.maxBlocks {
		return ErrCountAuditFull
	}
	return nil
}

// report records a mismatch.
func (a *countAuditor) report(block, problem string) {
	a.MismatchCount++
	if len(a.Mismatches) < countAuditReport {
		a.Mismatches = append(a.Mismatches, CountMismatch{Block: block, Problem: problem})
	}
}

// sumLists adds a holder for each child in the list of each holding block. A
// counted list whose block has no count record is a mismatch.
func (a *countAuditor) sumLists(ctx context.Context) error {
	return a.tx.ScanPrefix(ctx, []byte{kindChildren}, func(key, value []byte) error {
		// Read the block's count to learn whether it holds.
		if !a.counted(key) {
			return nil
		}
		block, _, err := readNode(key[1:])
		if err != nil {
			return err
		}
		rec, found, err := readCountTx(ctx, a.tx, block)
		if err != nil {
			return err
		}
		if !found {
			a.report(block, "child list has no count")
			return nil
		}
		if rec.count == 0 && !rec.holds {
			return nil
		}

		// Hold each child.
		children, err := decodeChildren(value)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := a.hold(child); err != nil {
				return err
			}
		}
		return nil
	})
}

// sumOwners adds a holder for each owner edge of a block from a node other
// than the unreferenced queue.
func (a *countAuditor) sumOwners(ctx context.Context) error {
	return a.tx.ScanPrefix(ctx, []byte{kindIncoming}, func(key, _ []byte) error {
		// Skip edges to nodes that are not blocks, and uncounted edges.
		if len(key) < 2 || key[1] == textNode || !a.counted(key) {
			return nil
		}

		// Hold the object of an owner edge.
		object, rest, err := readNode(key[1:])
		if err != nil {
			return err
		}
		subject, _, err := readNode(rest)
		if err != nil || subject == NodeUnreferenced {
			return err
		}
		return a.hold(object)
	})
}

// compareCounts compares each count record of the share with its summed
// holders, and checks that each zero count's queue entry names its block.
func (a *countAuditor) compareCounts(ctx context.Context) error {
	return a.tx.ScanPrefix(ctx, []byte{kindCount}, func(key, value []byte) error {
		// Decode a count record of the share.
		block, _, err := readNode(key[1:])
		if err != nil {
			return err
		}
		if !a.inShare(block) {
			return nil
		}
		rec, err := decodeCountRecord(value)
		if err != nil {
			return err
		}
		a.Blocks++

		// Compare the count with the holders.
		want := a.want[block]
		delete(a.want, block)
		if rec.count != want {
			a.report(block, "count "+strconv.FormatUint(rec.count, 10)+", "+strconv.FormatUint(want, 10)+" holders")
		}
		if rec.count != 0 {
			return nil
		}

		// Check the zero count's queue entry.
		entry, found, err := a.tx.Get(ctx, freeKey(rec.seq))
		if err != nil {
			return err
		}
		if !found || len(entry) < 8 {
			a.report(block, "zero count has no queue entry "+strconv.FormatUint(rec.seq, 10))
			return nil
		}
		if queued, _, err := readNode(entry[8:]); err != nil || queued != block {
			a.report(block, "queue entry "+strconv.FormatUint(rec.seq, 10)+" names another block")
		}
		return nil
	})
}

// compareQueue checks that each free queue entry of the share names a block
// whose count is zero and names the entry.
func (a *countAuditor) compareQueue(ctx context.Context) error {
	return a.tx.ScanPrefix(ctx, []byte{kindFree}, func(key, value []byte) error {
		// Decode an entry of the share.
		if len(key) != 9 || len(value) < 8 {
			return errors.Errorf("free queue entry %x does not decode", key)
		}
		block, _, err := readNode(value[8:])
		if err != nil {
			return err
		}
		if !a.inShare(block) {
			return nil
		}

		// Check that the block's zero count names the entry.
		seq := binary.BigEndian.Uint64(key[1:])
		rec, found, err := readCountTx(ctx, a.tx, block)
		if err != nil {
			return err
		}
		if !found || rec.count != 0 || rec.seq != seq {
			a.report(block, "queue entry "+strconv.FormatUint(seq, 10)+" without a zero count naming it")
		}
		return nil
	})
}
