//go:build !js

package cli_entrypoint

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestCliBusReleaseOrderAndExactOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var eventsMtx sync.Mutex
	var events []string
	record := func(event string) {
		eventsMtx.Lock()
		events = append(events, event)
		eventsMtx.Unlock()
	}
	busRelease := func(resource string) func() {
		return func() {
			select {
			case <-ctx.Done():
			default:
				t.Errorf("%s released before bus cancellation", resource)
			}
			record(resource)
		}
	}

	b := &CliBusImpl{
		cancel: func() {
			record("cancel")
			cancel()
		},
		busReleases: []func(){
			busRelease("controller"),
			busRelease("volume"),
			busRelease("world"),
		},
	}
	b.AddRelease(func() { record("state-lease") })

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			b.Release()
		})
	}
	wg.Wait()

	want := []string{"cancel", "world", "volume", "controller", "state-lease"}
	if !slices.Equal(events, want) {
		t.Fatalf("release order = %v, want %v", events, want)
	}
}

func TestBuildCliBusReleaseCancelsContext(t *testing.T) {
	b, err := BuildCliBus(
		context.Background(),
		logrus.New().WithField("test", t.Name()),
		t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}
	var callerReleases atomic.Int32
	b.AddRelease(func() { callerReleases.Add(1) })

	b.Release()
	b.Release()

	select {
	case <-b.GetContext().Done():
	default:
		t.Fatal("Release did not cancel the built bus context")
	}
	if got := callerReleases.Load(); got != 1 {
		t.Fatalf("caller release ran %d times, want 1", got)
	}
}
