package packahead

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/packfile"
)

// countRemote is a Remote that lists the packs in listed, holds what it is
// sent, and fails a PutPackFile while fail is set.
type countRemote struct {
	mtx    sync.Mutex
	listed map[string]bool
	fail   bool
}

func (r *countRemote) HasPackfile(_ context.Context, id string) (bool, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.listed[id], nil
}

func (r *countRemote) PutPackFile(_ context.Context, id string, _ []byte) (*packfile.PackfileEntry, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if r.fail {
		return nil, errors.New("put failed")
	}
	r.listed[id] = true
	return &packfile.PackfileEntry{Id: id}, nil
}

// TestDrainStatusCounts counts the packs a Drain writes, the ones the remote
// listed already, and a failed pass, with no pending pack left.
func TestDrainStatusCounts(t *testing.T) {
	ctx := context.Background()
	l, err := Open(ctx, t.TempDir(), Options{MaxPackBlocks: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// Finalize three packs of two blocks each.
	for i := range 6 {
		if err := l.Append(ctx, []Block{testBlock(t, i, 256)}); err != nil {
			t.Fatal(err)
		}
	}
	packs := waitFinalized(t, l, 2)

	// The remote lists the first pack and fails every put.
	remote := &countRemote{listed: map[string]bool{packs[0].ID: true}, fail: true}
	d := NewDrain(l, DrainOptions{})
	if err := d.Run(ctx, remote); err == nil {
		t.Fatal("run with a failing put succeeded")
	}
	st, _ := d.Status()
	if st.Failures != 1 || st.Listed != 1 || st.Written != 0 {
		t.Fatalf("after a failed pass: %+v; want 1 failure, 1 listed, 0 written", st)
	}

	// Drain the rest.
	remote.mtx.Lock()
	remote.fail = false
	remote.mtx.Unlock()
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- d.Run(runCtx, remote) }()
	deadline := time.After(10 * time.Second)
	for {
		st, changed := d.Status()
		if st.Pending == 0 && st.Written >= uint64(len(packs)-1) {
			if st.Listed != 1 || st.WrittenBytes == 0 {
				t.Fatalf("after the drain: %+v; want 1 listed and some bytes written", st)
			}
			break
		}
		select {
		case <-changed:
		case <-time.After(50 * time.Millisecond):
		case <-deadline:
			t.Fatalf("drain did not finish: %+v", st)
		}
	}
	cancel()
	<-done
	if st, _ := d.Status(); st.Failures != 1 {
		t.Fatalf("a canceled run counted a failure: %+v", st)
	}
}
