package packcopy

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

// countedTarget counts the reads a copy makes of its target.
type countedTarget struct {
	Target
	reads atomic.Int64
}

// GetStoredBlock counts the read and forwards it.
func (t *countedTarget) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	t.reads.Add(1)
	return t.Target.GetStoredBlock(ctx, ref)
}

// TestCopyHeldWithoutTargetReads checks that a pass over blocks the target
// holds but no proof covers reads them from the sources, writes nothing, and
// never reads the target; and that a held block no source serves is a missing
// hole unless ReadTarget is set.
func TestCopyHeldWithoutTargetReads(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	leaf := newTestBlock(t, "leaf")
	mid := newTestBlock(t, "mid", leaf)
	root := newTestBlock(t, "root", mid)
	blocks := []*testBlock{root, mid, leaf}

	bucket, client := s3test.New(t)
	source := block_store_s3.NewPackStore(le, client, s3test.BucketName, "src/", &block_store_s3.PackStoreOptions{KeepPacks: true})
	defer source.Close()
	var batch []*block.PutBatchEntry
	for _, b := range blocks {
		batch = append(batch, &block.PutBatchEntry{Ref: b.ref, Data: b.data, Refs: b.refs, RefsKnown: true})
	}
	if _, err := source.PutBlockBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}

	// Each pass has fresh proofs, so every block is held and unproven.
	index := store_kvtx_inmem.NewStore()
	pass := func(srcs []Source, readTarget bool) (*Result, *holeList, int64) {
		pack := block_store_s3.NewPackStore(le, client, s3test.BucketName, "v2/", &block_store_s3.PackStoreOptions{KeepPacks: true, Index: index})
		defer pack.Close()
		target := &countedTarget{Target: pack}
		holes := &holeList{reasons: make(map[string]string)}
		result, err := Copy(ctx, &Options{
			Roots:      []*block.BlockRef{root.ref},
			Target:     target,
			Sources:    srcs,
			Proofs:     store_kvtx_inmem.NewStore(),
			Holes:      holes.add,
			ReadTarget: readTarget,
		})
		if err != nil {
			t.Fatal(err)
		}
		return result, holes, target.reads.Load()
	}

	// The first pass writes every block.
	if result, _, _ := pass([]Source{source}, false); !result.Complete || result.Written != 3 {
		t.Fatalf("first pass: %+v", result)
	}

	// A held block comes from the source and is not written again.
	puts := bucket.Puts()
	result, _, reads := pass([]Source{source}, false)
	if !result.Complete || result.Held != 3 || result.Written != 0 || reads != 0 {
		t.Fatalf("held pass: %+v, %d target reads", result, reads)
	}
	if bucket.Puts() != puts {
		t.Fatal("a held pass wrote")
	}

	// With no source, the held root is a missing hole and the target is not read.
	result, holes, reads := pass(nil, false)
	if result.Complete || reads != 0 || holes.reasons[root.ref.MarshalString()] != ReasonMissing {
		t.Fatalf("sourceless pass: %+v, %d target reads, holes %v", result, reads, holes.reasons)
	}

	// ReadTarget reads it back from the target.
	result, _, reads = pass(nil, true)
	if !result.Complete || reads == 0 || result.Written != 0 {
		t.Fatalf("read-target pass: %+v, %d target reads", result, reads)
	}
}
