//go:build darwin || linux || windows

package s4db

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/routine"
)

// gatedPunch holds each hole punch until release closes, like a punch
// waiting on a busy disk.
type gatedPunch struct {
	storage
	entered chan struct{}
	once    sync.Once
	release chan struct{}
}

// punch reports the punch, then waits for release.
func (g *gatedPunch) punch(off, n int64) error {
	g.once.Do(func() { close(g.entered) })
	<-g.release
	return g.storage.punch(off, n)
}

// TestCommitDoesNotWaitForPunch checks that a single-process handle's commits
// proceed while a hole punch waits on the disk, and that the punched runs
// return to the free runs after it.
func TestCommitDoesNotWaitForPunch(t *testing.T) {
	// Open a file without the background routines, then gate its punches.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "punch.s4wave")
	db := openTest(t, path, Options{SingleProcess: true})
	for _, rc := range []*routine.RoutineContainer{db.warmer, db.cleaner} {
		if wait, _ := rc.SetRoutine(nil); wait != nil {
			<-wait
		}
	}
	g := &gatedPunch{storage: db.s, entered: make(chan struct{}), release: make(chan struct{})}
	db.s = g

	// Rewrite a batch of values until the space they free starts a punch.
	commit := func(round int) error {
		// Write every key with the round's value.
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		val := make([]byte, 4096)
		copy(val, fmt.Sprintf("%08d", round))
		for i := range 512 {
			_ = tx.Set(ctx, fmt.Appendf(nil, "k%04d", i), val)
		}
		return tx.Commit(ctx)
	}
	writes := make(chan error, 1)
	go func() {
		for round := 0; ; round++ {
			select {
			case <-g.entered:
				return
			default:
			}
			if err := commit(round); err != nil {
				writes <- err
				return
			}
		}
	}()
	select {
	case <-g.entered:
	case err := <-writes:
		t.Fatal(err)
	case <-time.After(30 * time.Second):
		close(g.release)
		t.Fatal("no hole punch started")
	}

	// A commit while the punch waits does not wait with it.
	done := make(chan error, 1)
	go func() { done <- commit(-1) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		close(g.release)
		<-done
		t.Fatal("a commit waited on the hole punch")
	}

	// The punch finishes and returns its runs to the free runs.
	close(g.release)
	db.w.punches.Wait()
	if err := db.w.lock(ctx); err != nil {
		t.Fatal(err)
	}
	reserved, punching := len(db.w.sp.reserved), db.w.punching.Load()
	db.w.unlock(nil)
	if reserved != 0 || punching {
		t.Fatalf("after the punch %d runs stay reserved, punching %v", reserved, punching)
	}

	// The reopened file holds the last commit.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openTest(t, path, Options{SingleProcess: true})
	defer db.Close()
	rtx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	v, ok, err := rtx.Get(ctx, []byte("k0000"))
	if err != nil || !ok || string(v[:8]) != fmt.Sprintf("%08d", -1) {
		t.Fatalf("get k0000 = %q %v %v", v[:min(len(v), 8)], ok, err)
	}
}
