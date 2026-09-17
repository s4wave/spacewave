package world_control_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// Journal-aware snapshot construction may wrap cancellation in storage context.
// A Wake still cancels only the current wait, not the owning controller.
func TestWatchLoopWakeAcceptsWrappedCancellation(t *testing.T) {
	for _, key := range []string{"", "object"} {
		t.Run("key="+key, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			tb, err := world_testbed.Default(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tb.Release)
			if key != "" {
				obj, err := tb.WorldState.CreateObject(ctx, key, nil)
				world.ReleaseObjectState(obj)
				if err != nil {
					t.Fatal(err)
				}
			}
			state := &wrappedCancelWaitState{WorldState: tb.WorldState, entered: make(chan struct{}, 1)}
			calls := 0
			loop := world_control.NewWatchLoop(nil, key, func(context.Context, *logrus.Entry, world.WorldState, world.ObjectState, *bucket.ObjectRef, uint64) (bool, error) {
				calls++
				return calls < 2, nil
			})
			done := make(chan error, 1)
			go func() { done <- loop.Execute(ctx, state) }()
			select {
			case <-state.entered:
			case <-ctx.Done():
				t.Fatal("watch never waited")
			}
			loop.Wake()
			select {
			case err := <-done:
				if err != nil || calls != 2 {
					t.Fatalf("wake ended controller: calls=%d err=%v", calls, err)
				}
			case <-ctx.Done():
				t.Fatal("wake did not continue the controller")
			}
		})
	}
}

func TestWatchLoopWaitStillPropagatesStorageFailure(t *testing.T) {
	failure := errors.New("journal corruption")
	state := &wrappedCancelWaitState{entered: make(chan struct{}, 1), failure: failure}
	loop := world_control.NewWatchLoop(nil, "", func(context.Context, *logrus.Entry, world.WorldState, world.ObjectState, *bucket.ObjectRef, uint64) (bool, error) {
		return true, nil
	})
	if err := loop.Execute(t.Context(), state); !errors.Is(err, failure) {
		t.Fatalf("lost storage failure: %v", err)
	}
}

type wrappedCancelWaitState struct {
	world.WorldState
	entered chan struct{}
	failure error
}

func (s *wrappedCancelWaitState) GetSeqno(context.Context) (uint64, error) { return 0, nil }
func (s *wrappedCancelWaitState) WaitSeqno(ctx context.Context, _ uint64) (uint64, error) {
	s.entered <- struct{}{}
	if s.failure != nil {
		return 0, fmt.Errorf("get gc journal sequence: %w", s.failure)
	}
	<-ctx.Done()
	return 0, fmt.Errorf("get gc journal sequence: %w", ctx.Err())
}

func (s *wrappedCancelWaitState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	obj, found, err := s.WorldState.GetObject(ctx, key)
	if obj == nil {
		return obj, found, err
	}
	return &wrappedCancelWaitObject{ObjectState: obj, state: s}, found, err
}

type wrappedCancelWaitObject struct {
	world.ObjectState
	state *wrappedCancelWaitState
}

func (o *wrappedCancelWaitObject) WaitRev(ctx context.Context, rev uint64, _ bool) (uint64, error) {
	return o.state.WaitSeqno(ctx, rev)
}
func (o *wrappedCancelWaitObject) Release() { world.ReleaseObjectState(o.ObjectState) }
