package space_world

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/sirupsen/logrus"
)

// stoppingSpace retains a real BodyHost claim across adapter startup cleanup.
type stoppingSpace struct {
	sobject.SharedObject
	// host owns the attached body's claim.
	host *sobject.BodyHost
	// cancel fails startup after the body attaches.
	cancel context.CancelFunc
	// stopped observes the point where the driver has joined.
	stopped chan struct{}
}

// SetBody cancels startup after claiming body and reports its joined stop.
func (s *stoppingSpace) SetBody(ctx context.Context, body sobject.Body) (func(), func(), error) {
	// Attach before canceling so Execute must unwind an acquired claim.
	stop, release, err := s.host.SetBody(ctx, body)
	if err != nil {
		return nil, nil, err
	}
	s.cancel()

	// Report the joined stop while Execute still retains its resources.
	notify := sync.OnceFunc(func() { close(s.stopped) })
	return func() { stop(); notify() }, release, nil
}

// TestAdapterStartupFailureRetainsClaim keeps replay and its claim until both
// the driver and background stores have stopped, including before opened.
func TestAdapterStartupFailureRetainsClaim(t *testing.T) {
	// Keep a real replay engine live while startup fails behind the root lock.
	r, start := newWorldReplay(t)
	createOn(t.Context(), t, r, start, "lifetime")
	a := r.a
	a.replay = r
	a.opened = make(chan struct{})
	unlock, err := a.rootMtx.Lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	so := &stoppingSpace{host: sobject.NewBodyHost(logrus.NewEntry(logrus.New()), nil, nil), cancel: cancel, stopped: make(chan struct{})}
	a.so = so
	doneStore, ok := a.startBackground()
	if !ok {
		t.Fatal("background store refused before startup")
	}
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() { defer close(finished); done <- a.Execute(ctx) }()
	t.Cleanup(func() {
		doneStore()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("startup cleanup did not finish")
		}
	})

	// The driver stops first, while background work still holds replay live.
	select {
	case <-so.stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("startup failure did not stop the body")
	}
	if r.engine == nil {
		t.Fatal("startup failure closed replay before draining background work")
	}
	if _, _, err := so.host.SetBody(ctx, &Adapter{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("successor during cleanup = %v, want canceled", err)
	}

	// Draining the last store closes replay and only then frees the claim.
	doneStore()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("startup failure = %v, want canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("startup failure never finished cleanup")
	}
	if r.engine != nil {
		t.Fatal("startup failure retained the replay engine")
	}
	_, release, err := so.host.SetBody(t.Context(), &Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	release()
}
