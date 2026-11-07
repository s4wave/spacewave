package s4db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/s4db/filelock"
)

// TestDarwinWriterIntent checks the ownership that avoids false kernel deadlocks.
func TestDarwinWriterIntent(t *testing.T) {
	// Keep an independent descriptor until after the engine closes, so its
	// eventual close cannot drop the engine's ordinary process-owned locks.
	path := filepath.Join(t.TempDir(), "intent.s4wave")
	probe, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	db := openTest(t, path, Options{})
	defer db.Close()
	db.sched.stop()

	// A different descriptor in the SAME process must conflict with intent.
	// A POSIX query ignores its own process locks but sees descriptor locks.
	if err := db.w.beginWait(); err != nil {
		t.Fatal(err)
	}
	defer db.w.endWait()
	held, err := filelock.Held(probe, lockForeground+int64(db.slot))
	if err != nil || !held {
		t.Fatalf("intent is not descriptor-owned: held=%t err=%v", held, err)
	}
}
