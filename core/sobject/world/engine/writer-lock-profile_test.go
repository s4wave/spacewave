//go:build !goscript

package sobject_world_engine

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

// TestWriterLockDumpsStuckHold checks that a hold past the stuck threshold
// writes the goroutine stacks once per interval and logs the file.
// GoScript deliberately does not expose native goroutine profiles.
func TestWriterLockDumpsStuckHold(t *testing.T) {
	// Log to a hook with short thresholds.
	logger, hook := test.NewNullLogger()
	l := writerLock{le: logrus.NewEntry(logger)}
	prevSlow, prevStuck, prevInterval := slowHoldThreshold, stuckHoldThreshold, stuckDumpInterval
	slowHoldThreshold, stuckHoldThreshold, stuckDumpInterval = time.Hour, 10*time.Millisecond, time.Hour
	t.Cleanup(func() { slowHoldThreshold, stuckHoldThreshold, stuckDumpInterval = prevSlow, prevStuck, prevInterval })
	ctx := context.Background()

	// Two stuck holds dump once, within the interval.
	for range 2 {
		unlock, err := l.Lock(ctx, "stuck-holder")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
		unlock()
	}
	entries := hook.AllEntries()
	if len(entries) != 1 {
		t.Fatalf("stuck holds logged %d entries, want 1", len(entries))
	}

	// Read the native profile from the path recorded by the diagnostic.
	path, _ := entries[0].Data["stacks"].(string)
	t.Cleanup(func() { _ = os.Remove(path) })
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "goroutine profile") {
		t.Fatalf("stacks file holds no goroutine profile: %.80q", b)
	}

	// A hold released before the threshold dumps nothing.
	stuckDumpInterval = 0
	unlock, err := l.Lock(ctx, "quick")
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	time.Sleep(30 * time.Millisecond)
	if n := len(hook.AllEntries()); n != 1 {
		t.Fatalf("quick hold logged, now %d entries", n)
	}
}
