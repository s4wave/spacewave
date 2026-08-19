package resource_space

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	"github.com/sirupsen/logrus"
)

func TestSpaceContentsResourceReleaseStopsRuntimeWaiter(t *testing.T) {
	r := NewSpaceContentsResource(nil, nil, nil, "space-test", "engine-test")
	runtime := &spaceRuntime{
		done:     make(chan struct{}),
		terminal: make(chan error),
	}
	runtimeReleased := make(chan struct{})
	runtime.schedulerRelease = func() { close(runtimeReleased) }
	r.startRuntime = func(
		_ context.Context,
		parent bus.Bus,
		_ *logrus.Entry,
		_ *plugin_space.Config,
	) (*spaceRuntime, error) {
		runtime.bus = parent
		return runtime, nil
	}
	ref := newTestSpaceContentsRef()
	r.startController = func(
		context.Context,
		bus.Bus,
		*plugin_space.Config,
	) (*plugin_space.Controller, directive.Reference, error) {
		return nil, ref, nil
	}

	waiterStarted := make(chan struct{})
	waiterExited := make(chan struct{})
	r.waitRuntimeTerminal = func(seq uint64, runtime *spaceRuntime, ctrlRef directive.Reference) {
		close(waiterStarted)
		r.runRuntimeTerminalWaiter(seq, runtime, ctrlRef)
		close(waiterExited)
	}

	r.StartController(&plugin_space.Config{})
	waitSpaceContentsCtrlRef(t, r, ref)
	select {
	case <-waiterStarted:
	case <-time.After(time.Second):
		t.Fatal("production runtime terminal waiter did not start")
	}
	r.Release()

	waitSpaceContentsRefReleased(t, ref, "resource release")
	select {
	case <-runtimeReleased:
	case <-time.After(time.Second):
		t.Fatal("runtime was not released")
	}
	select {
	case <-waiterExited:
	case <-time.After(time.Second):
		t.Fatal("runtime terminal waiter did not stop")
	}
}

func TestSpaceContentsResourceStartControllerReleasesReplacedController(t *testing.T) {
	r, calls := newTestSpaceContentsStartResource()
	defer r.Release()

	r.StartController(&plugin_space.Config{})
	first := recvSpaceContentsStartCall(t, calls, "first start")
	close(first.unblock)
	waitSpaceContentsCtrlRef(t, r, first.ref)

	r.StartController(&plugin_space.Config{})
	second := recvSpaceContentsStartCall(t, calls, "second start")
	close(second.unblock)
	waitSpaceContentsCtrlRef(t, r, second.ref)
	waitSpaceContentsRefReleased(t, first.ref, "first ref replacement")
	assertSpaceContentsRefNotReleased(t, second.ref, "second ref before release")

	r.Release()
	waitSpaceContentsRefReleased(t, second.ref, "second ref resource release")
}

func TestSpaceContentsResourceReplacementWatchesRuntimeTerminal(t *testing.T) {
	r := NewSpaceContentsResource(nil, nil, nil, "space-test", "engine-test")
	defer r.Release()

	runtimes := make(chan *spaceRuntime, 2)
	runtimeTerminals := make(chan chan error, 2)
	r.startRuntime = func(
		_ context.Context,
		parent bus.Bus,
		_ *logrus.Entry,
		_ *plugin_space.Config,
	) (*spaceRuntime, error) {
		terminal := make(chan error, 1)
		runtime := &spaceRuntime{
			bus:      parent,
			done:     make(chan struct{}),
			terminal: terminal,
		}
		runtimes <- runtime
		runtimeTerminals <- terminal
		return runtime, nil
	}
	refs := make(chan *testSpaceContentsRef, 2)
	r.startController = func(
		context.Context,
		bus.Bus,
		*plugin_space.Config,
	) (*plugin_space.Controller, directive.Reference, error) {
		ref := newTestSpaceContentsRef()
		refs <- ref
		return nil, ref, nil
	}

	r.StartController(&plugin_space.Config{})
	firstRuntime := <-runtimes
	<-runtimeTerminals
	firstRef := <-refs
	waitSpaceContentsCtrlRef(t, r, firstRef)

	r.StartController(&plugin_space.Config{})
	<-runtimes
	secondTerminal := <-runtimeTerminals
	secondRef := <-refs
	waitSpaceContentsCtrlRef(t, r, secondRef)
	waitSpaceContentsRefReleased(t, firstRef, "replaced ref")
	select {
	case <-firstRuntime.done:
	case <-time.After(time.Second):
		t.Fatal("replaced runtime was not released")
	}

	wantErr := errors.New("replacement runtime failed")
	secondTerminal <- wantErr
	waitSpaceContentsRefReleased(t, secondRef, "failed replacement ref")
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := r.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return errors.Is(r.startErr, wantErr) && r.ctrlRef == nil && r.runtime == nil, nil
	}); err != nil {
		t.Fatalf("replacement runtime failure was not projected: %v", err)
	}
}

func TestSpaceContentsResourceStartControllerReleasesStaleStartup(t *testing.T) {
	r, calls := newTestSpaceContentsStartResource()
	defer r.Release()

	r.StartController(&plugin_space.Config{})
	first := recvSpaceContentsStartCall(t, calls, "first start")

	r.StartController(&plugin_space.Config{})
	select {
	case <-first.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("first startup was not canceled by replacement")
	}
	first.returnOnCancel()
	waitSpaceContentsRefReleased(t, first.ref, "stale first ref")

	second := recvSpaceContentsStartCall(t, calls, "second start")
	close(second.unblock)
	waitSpaceContentsCtrlRef(t, r, second.ref)
	assertSpaceContentsRefNotReleased(t, second.ref, "second ref before release")
}

func TestSpaceContentsResourceReleaseCancelsStartupAndReleasesLateRef(t *testing.T) {
	r, calls := newTestSpaceContentsStartResource()

	r.StartController(&plugin_space.Config{})
	call := recvSpaceContentsStartCall(t, calls, "start")
	r.Release()

	select {
	case <-call.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("startup was not canceled by release")
	}
	call.returnOnCancel()
	waitSpaceContentsRefReleased(t, call.ref, "late startup ref")
	assertSpaceContentsCtrlRef(t, r, nil)

	select {
	case extra := <-calls:
		t.Fatalf("unexpected startup after release: %#v", extra)
	case <-time.After(20 * time.Millisecond):
	}
}

type testSpaceContentsStartCall struct {
	ctx     context.Context
	ref     *testSpaceContentsRef
	unblock chan struct{}
	once    sync.Once
}

func (c *testSpaceContentsStartCall) returnOnCancel() {
	c.once.Do(func() {
		close(c.unblock)
	})
}

type testSpaceContentsRef struct {
	once     sync.Once
	released chan struct{}
}

func newTestSpaceContentsRef() *testSpaceContentsRef {
	return &testSpaceContentsRef{released: make(chan struct{})}
}

func (r *testSpaceContentsRef) Release() {
	r.once.Do(func() {
		close(r.released)
	})
}

func newTestSpaceContentsStartResource() (*SpaceContentsResource, <-chan *testSpaceContentsStartCall) {
	r := NewSpaceContentsResource(nil, nil, nil, "space-test", "engine-test")
	calls := make(chan *testSpaceContentsStartCall, 4)
	r.startRuntime = func(
		_ context.Context,
		parent bus.Bus,
		_ *logrus.Entry,
		_ *plugin_space.Config,
	) (*spaceRuntime, error) {
		return &spaceRuntime{bus: parent, done: make(chan struct{})}, nil
	}
	r.startController = func(
		ctx context.Context,
		_ bus.Bus,
		_ *plugin_space.Config,
	) (*plugin_space.Controller, directive.Reference, error) {
		call := &testSpaceContentsStartCall{
			ctx:     ctx,
			ref:     newTestSpaceContentsRef(),
			unblock: make(chan struct{}),
		}
		calls <- call
		select {
		case <-ctx.Done():
		case <-call.unblock:
		}
		return nil, call.ref, nil
	}
	return r, calls
}

func recvSpaceContentsStartCall(
	t *testing.T,
	calls <-chan *testSpaceContentsStartCall,
	name string,
) *testSpaceContentsStartCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
		return nil
	}
}

func waitSpaceContentsCtrlRef(t *testing.T, r *SpaceContentsResource, want directive.Reference) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := r.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return r.ctrlRef == want, nil
	}); err != nil {
		t.Fatalf("timed out waiting for ctrlRef %#v: %v", want, err)
	}
}

func assertSpaceContentsCtrlRef(t *testing.T, r *SpaceContentsResource, want directive.Reference) {
	t.Helper()
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if r.ctrlRef != want {
			t.Fatalf("expected ctrlRef %#v, got %#v", want, r.ctrlRef)
		}
	})
}

func waitSpaceContentsRefReleased(t *testing.T, ref *testSpaceContentsRef, name string) {
	t.Helper()
	select {
	case <-ref.released:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s release", name)
	}
}

func assertSpaceContentsRefNotReleased(t *testing.T, ref *testSpaceContentsRef, name string) {
	t.Helper()
	select {
	case <-ref.released:
		t.Fatalf("%s released unexpectedly", name)
	default:
	}
}
