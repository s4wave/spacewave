package sobject_world_engine

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

// TestWriterLockLogsSlowHold checks that releasing the writer lock after a
// slow hold logs the holder once, and that a short hold logs nothing.
func TestWriterLockLogsSlowHold(t *testing.T) {
	// Log to a hook with a short threshold.
	logger, hook := test.NewNullLogger()
	l := writerLock{le: logrus.NewEntry(logger)}
	prev := slowHoldThreshold
	slowHoldThreshold = 20 * time.Millisecond
	t.Cleanup(func() { slowHoldThreshold = prev })
	ctx := context.Background()

	// A short hold logs nothing.
	unlock, err := l.Lock(ctx, "quick")
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	if n := len(hook.AllEntries()); n != 0 {
		t.Fatalf("short hold logged %d entries", n)
	}

	// A slow hold released twice logs its holder once.
	unlock, err = l.Lock(ctx, "slow-holder")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	unlock()
	unlock()

	// It logged one entry naming the holder and the hold.
	entries := hook.AllEntries()
	if len(entries) != 1 {
		t.Fatalf("slow hold logged %d entries, want 1", len(entries))
	}
	if h := entries[0].Data["holder"]; h != "slow-holder" {
		t.Fatalf("holder %v, want slow-holder", h)
	}
	if held := entries[0].Data["held"].(time.Duration); held < 30*time.Millisecond {
		t.Fatalf("held %s, want at least 30ms", held)
	}

	// The lock is free again.
	unlock, err = l.Lock(ctx, "after")
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}
