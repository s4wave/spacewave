//go:build !js && !bldr_sqlite

package devtool

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pkg/errors"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/sirupsen/logrus"
)

// TestBuildDevtoolBusReopenAfterInProcessOpen checks readiness through the full
// devtool startup path after a retained database's conflicting claim clears.
func TestBuildDevtoolBusReopenAfterInProcessOpen(t *testing.T) {
	// Bound both boots and every claim wait independently of the suite timeout.
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	t.Cleanup(cancel)
	root := t.TempDir()
	stateRoot := filepath.Join(root, ".bldr")
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Boot and release the devtool against an isolated retained volume.
	first, err := BuildDevtoolBus(ctx, le, root, stateRoot, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(first.Release)
	firstID := first.vol.GetID()
	first.Release()
	if err := first.ctx.Err(); err != context.Canceled {
		t.Fatalf("released devtool context: %v, want cancellation", err)
	}

	// Claim the retained file after the first controller finishes closing it.
	path, err := storage_native.VolumePath(stateRoot, "devtool")
	if err != nil {
		t.Fatal(err)
	}
	claim := claimDevtoolDatabase(t, ctx, path)
	hook := &reopenClaimHook{
		claim:  make(chan *s4db.DB, 1),
		closed: make(chan error, 1),
	}
	hook.claim <- claim
	t.Cleanup(func() {
		select {
		case db := <-hook.claim:
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		default:
		}
	})
	log.AddHook(hook)

	// Reopen through ExecVolumeController and GetVolume inside BuildDevtoolBus.
	second, err := BuildDevtoolBus(ctx, le, root, stateRoot, false, nil)
	if err != nil {
		t.Fatalf("reopen devtool after conflicting claim: %v", err)
	}
	t.Cleanup(second.Release)
	if got := second.vol.GetID(); got != firstID {
		t.Fatalf("reopened volume id %q, want %q", got, firstID)
	}

	// Require the conflicting open to have occurred and released the claim.
	select {
	case err := <-hook.closed:
		if err != nil {
			t.Fatalf("release conflicting claim: %v", err)
		}
	default:
		t.Fatal("devtool reopened without encountering the retained claim")
	}
}

// claimDevtoolDatabase waits for the released controller to relinquish its file.
func claimDevtoolDatabase(t *testing.T, ctx context.Context, path string) *s4db.DB {
	t.Helper()

	// Wait on the database claim event instead of racing asynchronous teardown.
	for {
		db, err := s4db.Open(path, s4db.Options{})
		if err == nil {
			return db
		}
		var waiter interface {
			error
			Wait(context.Context) error
		}
		if !errors.Is(err, s4db.ErrOpenInProcess) || !errors.As(err, &waiter) {
			t.Fatal(err)
		}
		if err := waiter.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
}
