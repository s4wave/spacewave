package block_gc

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

const (
	driveStorageBenchmarkBlocks = 139
	driveStorageBenchmarkRefs   = driveStorageBenchmarkBlocks - 1
)

func BenchmarkDriveRatioWriteAtRootStorage(b *testing.B) {
	ctx := context.Background()
	var writeAtRootDuration time.Duration
	var flushPendingDuration time.Duration
	var applyRefBatchDuration time.Duration
	var cayleyTransactions int

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		store, refGraph := newDriveRatioBenchmarkStore(b, ctx)
		tx := newDriveRatioBenchmarkTransaction(store)
		b.StartTimer()

		start := time.Now()
		if _, _, err := tx.Write(ctx, true); err != nil {
			b.Fatal(err)
		}
		writeAtRootDuration += time.Since(start)

		b.StopTimer()
		flushPendingDuration += store.flushDuration
		applyRefBatchDuration += refGraph.applyRefBatchDuration
		cayleyTransactions += refGraph.cayleyTransactions
		refGraph.Close()
	}

	ops := float64(b.N)
	b.ReportMetric(float64(writeAtRootDuration.Microseconds())/ops, "write_at_root_us/op")
	b.ReportMetric(float64(flushPendingDuration.Microseconds())/ops, "flush_pending_us/op")
	b.ReportMetric(float64(applyRefBatchDuration.Microseconds())/ops, "apply_ref_batch_us/op")
	b.ReportMetric(driveStorageBenchmarkBlocks, "blocks/op")
	b.ReportMetric(driveStorageBenchmarkRefs, "ref_edges/op")
	b.ReportMetric(float64(cayleyTransactions)/ops, "cayley_transactions/op")
}

func newDriveRatioBenchmarkStore(
	b *testing.B,
	ctx context.Context,
) (*driveRatioBenchmarkStore, *driveRatioBenchmarkRefGraph) {
	b.Helper()

	kvStore := store_kvtx_inmem.NewStore()
	rawStore := block_store_kvtx.NewKVTxBlock(store_kvkey.NewDefaultKVKey(), kvStore, 0, false)
	refGraph, err := NewRefGraph(ctx, kvStore, []byte("gc/"))
	if err != nil {
		b.Fatal(err)
	}
	timedGraph := &driveRatioBenchmarkRefGraph{RefGraph: refGraph}
	return &driveRatioBenchmarkStore{GCStoreOps: NewGCStoreOps(rawStore, timedGraph)}, timedGraph
}

func newDriveRatioBenchmarkTransaction(store block.StoreOps) *block.Transaction {
	tx, cursor := block.NewTransaction(store, nil, nil, nil)
	for i := range driveStorageBenchmarkBlocks {
		cursor.SetBlock(newDriveRatioBenchmarkBlock(i), true)
		if i != driveStorageBenchmarkBlocks-1 {
			cursor = cursor.FollowRef(1, nil)
		}
	}
	return tx
}

type driveRatioBenchmarkBlock struct {
	msg  string
	refs map[uint32]*block.BlockRef
}

func newDriveRatioBenchmarkBlock(i int) *driveRatioBenchmarkBlock {
	return &driveRatioBenchmarkBlock{msg: "drive-ratio-block-" + strconv.Itoa(i)}
}

func (blk *driveRatioBenchmarkBlock) MarshalBlock() ([]byte, error) {
	return []byte(blk.msg), nil
}

func (blk *driveRatioBenchmarkBlock) UnmarshalBlock(data []byte) error {
	blk.msg = string(data)
	return nil
}

func (blk *driveRatioBenchmarkBlock) ApplyBlockRef(id uint32, ref *block.BlockRef) error {
	if blk.refs == nil {
		blk.refs = make(map[uint32]*block.BlockRef)
	}
	blk.refs[id] = ref
	return nil
}

func (blk *driveRatioBenchmarkBlock) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return blk.refs, nil
}

func (blk *driveRatioBenchmarkBlock) GetBlockRefCtor(uint32) block.Ctor {
	return nil
}

type driveRatioBenchmarkStore struct {
	*GCStoreOps

	flushDuration time.Duration
}

func (s *driveRatioBenchmarkStore) EndDeferFlush(ctx context.Context) error {
	start := time.Now()
	err := s.GCStoreOps.EndDeferFlush(ctx)
	s.flushDuration += time.Since(start)
	return err
}

type driveRatioBenchmarkRefGraph struct {
	*RefGraph

	applyRefBatchDuration time.Duration
	cayleyTransactions    int
}

func (rg *driveRatioBenchmarkRefGraph) ApplyRefBatch(ctx context.Context, adds, removes []RefEdge) error {
	start := time.Now()
	err := rg.RefGraph.ApplyRefBatch(ctx, adds, removes)
	rg.applyRefBatchDuration += time.Since(start)
	rg.cayleyTransactions += refBatchTransactions(len(adds)) + refBatchTransactions(len(removes))
	return err
}

func refBatchTransactions(edges int) int {
	if edges == 0 {
		return 0
	}
	return (edges + refGraphApplyBatchLimit - 1) / refGraphApplyBatchLimit
}
