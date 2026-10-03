package statusprojector

import (
	"fmt"
	"testing"
	"time"
)

func TestWaitAnyStatusChangeWaitsForEverySource(t *testing.T) {
	t.Parallel()

	for sourceIdx := range 5 {
		t.Run(fmt.Sprintf("source-%d", sourceIdx), func(t *testing.T) {
			// Run each status source check concurrently.
			t.Parallel()

			// Start a waiter over all status source notifications.
			waitChs, closeSource := testWaitChannels(5, sourceIdx)
			done := make(chan bool, 1)
			go func() {
				done <- waitAnyStatusChange(t.Context(), waitChs)
			}()

			// Verify the status waiter remains blocked before any source changes.
			select {
			case ctxDone := <-done:
				t.Fatalf("waitAnyStatusChange returned before source %d woke: %v", sourceIdx, ctxDone)
			case <-time.After(10 * time.Millisecond):
			}

			// Notify the status waiter through the selected source.
			closeSource()

			// Verify the selected source wakes the status waiter.
			select {
			case ctxDone := <-done:
				if ctxDone {
					t.Fatalf("waitAnyStatusChange reported context done after source %d woke", sourceIdx)
				}
			case <-time.After(time.Second):
				t.Fatalf("waitAnyStatusChange ignored source %d", sourceIdx)
			}
		})
	}
}

func testWaitChannels(count int, sourceIdx int) ([]<-chan struct{}, func()) {
	// Build wait channels with absent sources around the active sources.
	chans := make([]chan struct{}, count)
	waitChs := make([]<-chan struct{}, 0, count+2)
	waitChs = append(waitChs, nil)
	for i := range chans {
		chans[i] = make(chan struct{})
		waitChs = append(waitChs, chans[i])
	}
	waitChs = append(waitChs, nil)
	return waitChs, func() {
		close(chans[sourceIdx])
	}
}
