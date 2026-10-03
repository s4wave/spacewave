package resource_root

import (
	"fmt"
	"testing"
	"time"
)

func TestWaitAnyWaitsForEverySource(t *testing.T) {
	t.Parallel()

	for sourceIdx := range 5 {
		t.Run(fmt.Sprintf("source-%d", sourceIdx), func(t *testing.T) {
			// Run each Session wait source independently.
			t.Parallel()

			// Start a wait across all Session change sources.
			waitChs, closeSource := testWaitChannels(5, sourceIdx)
			done := make(chan bool, 1)
			go func() {
				done <- waitAny(t.Context(), waitChs)
			}()

			// Verify the Session wait remains blocked before a source changes.
			select {
			case ctxDone := <-done:
				t.Fatalf("waitAny returned before source %d woke: %v", sourceIdx, ctxDone)
			case <-time.After(10 * time.Millisecond):
			}

			// Notify the selected Session change source.
			closeSource()

			// Verify the Session wait reports the source change without cancellation.
			select {
			case ctxDone := <-done:
				if ctxDone {
					t.Fatalf("waitAny reported context done after source %d woke", sourceIdx)
				}
			case <-time.After(time.Second):
				t.Fatalf("waitAny ignored source %d", sourceIdx)
			}
		})
	}
}

func testWaitChannels(count int, sourceIdx int) ([]<-chan struct{}, func()) {
	// Build Session wait sources with nil channels around the active set.
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
