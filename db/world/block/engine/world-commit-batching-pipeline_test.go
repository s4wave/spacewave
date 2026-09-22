//go:build !js && !wasip1

package world_block_engine_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
)

// TestWorldCommitBatchingResourceRunAhead is a wire-level contract check. A
// physical writer turn blocks persistence, not preparation of the next World
// revision. No private engine, concrete Submit call, or alternate block route
// participates in the Resource operations under test.
func TestWorldCommitBatchingResourceRunAhead(t *testing.T) {
	f := newBatchingFixture(t, 512)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	initial, err := f.engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Discard(context.Background()); first.Release() })
	// The first coordinator refresh must precede the blocked physical turn.
	// Reused same-World authority must not remap the database again while N is
	// pending. This barrier does not change the sync/freelist configuration.
	physical, err := f.db.Begin(true)
	if err != nil {
		t.Fatal(err)
	}
	defer physical.Rollback()
	before := f.db.CommitCounter()
	publications := f.tb.Volume.(interface {
		GetPublicationStats() volume_kvtx.PublicationStats
	})
	beforePublications := publications.GetPublicationStats().PhysicalCommits
	keys1 := batchingResourcePopulate(t, ctx, f, first, 0, 32)
	done1 := make(chan error, 1)
	go func() { done1 <- first.Commit(ctx) }()
	second, err := f.engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("successor could not prepare before persistence: %v", err)
	}
	t.Cleanup(func() { _ = second.Discard(context.Background()); second.Release() })
	obj, found, err := second.GetObject(ctx, keys1[0])
	world.ReleaseObjectState(obj)
	if err != nil || !found {
		t.Fatalf("successor did not inherit private revision: %v %v", found, err)
	}
	keys2 := batchingResourcePopulate(t, ctx, f, second, 1, 32)
	done2 := make(chan error, 1)
	go func() { done2 <- second.Commit(ctx) }()
	// Admission of a following transaction proves the second commit sealed and
	// released its producer turn, rather than merely starting a goroutine.
	probe, err := f.engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	obj, found, err = probe.GetObject(ctx, keys2[31])
	world.ReleaseObjectState(obj)
	if err != nil || !found {
		t.Fatalf("second private revision missing: %v %v", found, err)
	}
	if err := probe.Discard(ctx); err != nil {
		probe.Release()
		t.Fatal(err)
	}
	probe.Release()
	seq, err := f.engine.GetSeqno(ctx)
	if err != nil || seq != initial {
		t.Fatalf("canonical seqno advanced before durability: %d want %d, %v", seq, initial, err)
	}
	for _, ch := range []<-chan error{done1, done2} {
		select {
		case err := <-ch:
			t.Fatalf("Commit acknowledged before durability: %v", err)
		default:
		}
	}
	if n := f.db.CommitCounter() - before; n != 0 {
		t.Fatalf("construction wrote %d physical commits", n)
	}
	if err := physical.Rollback(); err != nil {
		t.Fatal(err)
	}
	for _, ch := range []<-chan error{done1, done2} {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if n := publications.GetPublicationStats().PhysicalCommits - beforePublications; n != 1 {
		t.Fatalf("two Resource revisions used %d publication commits, want 1", n)
	}
	// Reader pins and their release are separate ownership transactions.
	t.Logf("physical commits including reader ownership: %d", f.db.CommitCounter()-before)
	p1 := batchingResourceReadback(t, ctx, f, keys1)
	p2 := batchingResourceReadback(t, ctx, f, keys2)
	if p1 != 3919745061 || p2 != 3325196325 {
		t.Fatalf("content/relationship parity changed: %d %d", p1, p2)
	}
}

// This benchmark measures natural occupancy without adding an artificial
// coalescing delay. Complete update throughput and per-caller acknowledgment
// latency are different metrics; both are reported with physical commit counts.
func BenchmarkWorldCommitBatchingResourcePipeline(b *testing.B) {
	for _, objects := range []int{1, 32} {
		b.Run(fmt.Sprintf("objects=%d", objects), func(b *testing.B) {
			f := newBatchingFixture(b, 512)
			const window = 8
			ctx := b.Context()
			b.ReportAllocs()
			b.ResetTimer()
			before := f.db.CommitCounter()
			start := time.Now()
			var pending []<-chan pipelineResult
			var keys [][]string
			var elapsed time.Duration
			join := func() {
				got := <-pending[0]
				pending = pending[1:]
				if got.err != nil {
					b.Fatal(got.err)
				}
				elapsed += got.elapsed
			}
			for i := 0; i < b.N; i++ {
				if len(pending) == window {
					join()
				}
				w, err := f.engine.NewTransaction(ctx, true)
				if err != nil {
					b.Fatal(err)
				}
				keys = append(keys, batchingResourcePopulate(b, ctx, f, w, i, objects))
				done := make(chan pipelineResult, 1)
				pending = append(pending, done)
				go func(w *sdk_world.Tx) {
					at := time.Now()
					err := w.Commit(ctx)
					latency := time.Since(at)
					_ = w.Discard(context.Background())
					w.Release()
					done <- pipelineResult{latency, err}
				}(w)
			}
			for len(pending) != 0 {
				join()
			}
			total := time.Since(start)
			physical := f.db.CommitCounter() - before
			b.StopTimer()
			for _, ks := range keys {
				batchingResourceReadback(b, ctx, f, ks)
			}
			b.ReportMetric(float64(b.N)/total.Seconds(), "durable-updates/s")
			b.ReportMetric(float64(elapsed.Nanoseconds())/float64(b.N), "ack-ns/op")
			b.ReportMetric(float64(physical)/float64(b.N), "physical-commits/op")
			if physical != 0 {
				b.ReportMetric(float64(b.N)/float64(physical), "updates/physical-commit")
			}
		})
	}
}

type pipelineResult struct {
	elapsed time.Duration
	err     error
}
