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
			t.Parallel()

			waitChs, closeSource := testWaitChannels(5, sourceIdx)
			done := make(chan bool, 1)
			go func() {
				done <- waitAnyStatusChange(t.Context(), waitChs)
			}()

			select {
			case ctxDone := <-done:
				t.Fatalf("waitAnyStatusChange returned before source %d woke: %v", sourceIdx, ctxDone)
			case <-time.After(10 * time.Millisecond):
			}

			closeSource()

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
