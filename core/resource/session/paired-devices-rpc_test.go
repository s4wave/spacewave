package resource_session

import (
	"fmt"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
)

func TestWaitPairedDevicesWaitsForEverySource(t *testing.T) {
	t.Parallel()

	for sourceIdx := range 5 {
		sourceIdx := sourceIdx
		t.Run(fmt.Sprintf("source-%d", sourceIdx), func(t *testing.T) {
			t.Parallel()

			waitChs, closeSource := testWaitChannels(5, sourceIdx)
			stateCh := make(chan sobject.SharedObjectStateSnapshot)
			errCh := make(chan error, 1)
			done := make(chan error, 1)
			go func() {
				_, err := waitPairedDevices(t.Context(), stateCh, errCh, waitChs)
				done <- err
			}()

			select {
			case err := <-done:
				t.Fatalf("waitPairedDevices returned before source %d woke: %v", sourceIdx, err)
			case <-time.After(10 * time.Millisecond):
			}

			closeSource()

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("waitPairedDevices returned %v after source %d woke", err, sourceIdx)
				}
			case <-time.After(time.Second):
				t.Fatalf("waitPairedDevices ignored source %d", sourceIdx)
			}
		})
	}
}
