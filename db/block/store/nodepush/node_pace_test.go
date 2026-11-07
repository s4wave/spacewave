package block_store_nodepush

import (
	"context"
	"testing"
	"time"
)

// TestNodeCopiesOffAndUploadPace turns read copies off and checks that an
// uploaded pack is not kept and a read miss queues no fetch, then that the
// upload's pace waits out each pack's share and that no pace waits nothing.
func TestNodeCopiesOffAndUploadPace(t *testing.T) {
	// Copies off: Keep stores nothing, and a miss queues no fill.
	packs := newPackFiles(t, t.TempDir(), &objectRemote{})
	packs.SetCopies(false)
	packs.Keep("p1", []byte("pack bytes"))
	packs.queueFill("p2")
	if st := packs.Stats(); st.Kept != 0 || len(packs.fills) != 0 {
		t.Fatalf("copies off kept %d packs and queued %d fills", st.Kept, len(packs.fills))
	}

	// Back on, a miss queues its fill again.
	packs.SetCopies(true)
	packs.queueFill("p2")
	if len(packs.fills) != 1 {
		t.Fatal("copies back on queued no fill")
	}

	// The pace waits a pack's share: 1 MiB at 10 MiB/s is 100 ms.
	n := &Node{}
	n.SetUploadRate(10 << 20)
	start := time.Now()
	if err := n.admitUpload(context.Background(), 1<<20); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 90*time.Millisecond || waited > 2*time.Second {
		t.Fatalf("paced upload waited %v, want about 100 ms", waited)
	}

	// No pace waits nothing.
	n.SetUploadRate(0)
	start = time.Now()
	if err := n.admitUpload(context.Background(), 1<<30); err != nil || time.Since(start) > 50*time.Millisecond {
		t.Fatalf("unpaced upload waited %v (%v)", time.Since(start), err)
	}
}
