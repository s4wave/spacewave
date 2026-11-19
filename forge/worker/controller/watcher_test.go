package worker_controller

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/identity"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// loopRuns counts the Worker controller's handler runs.
type loopRuns struct{ n atomic.Int64 }

func (*loopRuns) Levels() []logrus.Level { return logrus.AllLevels }

func (r *loopRuns) Fire(e *logrus.Entry) error {
	if e.Data["object-loop"] == "worker-controller" && strings.HasPrefix(e.Message, "object found at rev") {
		r.n.Add(1)
	}
	return nil
}

// settle waits until the loop has not run for 200 ms and returns its runs.
func (r *loopRuns) settle() int64 {
	last := r.n.Load()
	for {
		time.Sleep(200 * time.Millisecond)
		now := r.n.Load()
		if now == last {
			return now
		}
		last = now
	}
}

// TestWorkerIgnoresUnrelatedWrites verifies that the Worker controller runs
// again when its keypair's assignments change, and not for an unrelated
// World write.
func TestWorkerIgnoresUnrelatedWrites(t *testing.T) {
	// A Worker with one keypair that 50 objects are linked to.
	const nObjects, nWrites = 50, 100
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	workerPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	kp, err := identity.NewKeypair(workerPeer.GetPubKey(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_worker.CreateWorker(ctx, tb.WorldState, "worker/1", "worker", []*identity.Keypair{kp}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}
	kpKey := identity_world.NewKeypairKey(workerPeer.GetPeerID().String())
	link := func(key string) {
		body := []byte(key)
		if _, _, err := world.AccessWorldObject(ctx, tb.WorldState, key, true, func(bcs *block.Cursor) error {
			bcs.SetBlock(byteslice.NewByteSlice(&body), true)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := tb.WorldState.SetGraphQuad(ctx, identity_world.NewObjectToKeypairQuad(key, kpKey)); err != nil {
			t.Fatal(err)
		}
	}
	for i := range nObjects {
		link(fmt.Sprintf("assigned/%03d", i))
	}

	// writeUnrelated writes an unrelated object nWrites times and returns the
	// bytes allocated per write.
	writeUnrelated := func(key string) uint64 {
		var ms runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&ms)
		alloc := ms.TotalAlloc
		for i := range nWrites {
			body := []byte(fmt.Sprintf("v%d", i))
			if _, _, err := world.AccessWorldObject(ctx, tb.WorldState, key, true, func(bcs *block.Cursor) error {
				bcs.SetBlock(byteslice.NewByteSlice(&body), true)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
		runtime.ReadMemStats(&ms)
		return (ms.TotalAlloc - alloc) / nWrites
	}
	alone := writeUnrelated("unrelated-alone")

	// Run the controller with a logger that counts its handler runs.
	runs := &loopRuns{}
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	log.SetOutput(io.Discard)
	log.AddHook(runs)
	c := NewController(logrus.NewEntry(log), tb.Bus, NewConfig(tb.EngineID, "worker/1", workerPeer.GetPeerID(), false))
	go func() { _ = c.Execute(ctx) }()
	for runs.n.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	start := runs.settle()

	// Unrelated object writes run no handler.
	perWrite := writeUnrelated("unrelated")
	after := runs.settle()
	t.Logf("unrelated object write: %.2f handler runs, %d KiB allocated per write (%d KiB with no controller)",
		float64(after-start)/nWrites, perWrite>>10, alone>>10)
	if after != start {
		t.Fatalf("unrelated writes ran the handler %d times", after-start)
	}

	// A new assignment runs it again.
	link("assigned/new")
	if runs.settle() == after {
		t.Fatal("a new assignment did not run the handler")
	}
}
