package cluster_controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestSkipUnhandledOperation(t *testing.T) {
	handler := skipUnhandledOperation(func(
		context.Context,
		*logrus.Entry,
		world.WorldState,
		world.ObjectState,
		*bucket.ObjectRef,
		uint64,
	) (bool, error) {
		return false, world.ErrUnhandledOp
	})

	waitForChanges, err := handler(context.Background(), nil, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if !waitForChanges {
		t.Fatal("waitForChanges = false, want true")
	}
}

func TestJobTrackerRetriesTransientWorldError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	controller := NewController(
		tb.Logger,
		tb.Bus,
		NewConfig(tb.EngineID, "test-cluster", tb.Volume.GetPeerID()),
	)
	tracker, _ := controller.jobTrackers.SetKey("test-job", false)

	attempts := make(chan int, 2)
	attempt := 0
	tracker.objLoop = world_control.NewWatchLoop(
		tb.Logger,
		"",
		func(
			context.Context,
			*logrus.Entry,
			world.WorldState,
			world.ObjectState,
			*bucket.ObjectRef,
			uint64,
		) (bool, error) {
			attempt++
			attempts <- attempt
			if attempt == 1 {
				return false, errors.New("transient world read")
			}
			return false, nil
		},
	)

	controller.jobTrackers.SetContext(ctx, true)
	for want := 1; want <= 2; want++ {
		select {
		case got := <-attempts:
			if got != want {
				t.Fatalf("attempt = %d, want %d", got, want)
			}
		case <-ctx.Done():
			t.Fatalf("job tracker attempt %d did not start: %v", want, ctx.Err())
		}
	}
}
