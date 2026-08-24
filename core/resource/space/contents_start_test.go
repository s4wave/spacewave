package resource_space

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
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
	r.waitRuntimeTerminal = func(seq uint64, runtime *spaceRuntime, ctrlRef directive.Reference, conf *plugin_space.Config) {
		close(waiterStarted)
		r.runRuntimeTerminalWaiter(seq, runtime, ctrlRef, conf)
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

func TestSpaceContentsResourceReleasePreventsHostRuntimeRestart(t *testing.T) {
	r := NewSpaceContentsResource(nil, nil, nil, "space-test", "engine-test")
	terminal := make(chan error, 1)
	runtime := &spaceRuntime{terminal: terminal}
	startCalls := make(chan *plugin_space.Config, 2)
	ref := newTestSpaceContentsRef()
	r.startRuntime = func(
		_ context.Context,
		parent bus.Bus,
		_ *logrus.Entry,
		conf *plugin_space.Config,
	) (*spaceRuntime, error) {
		startCalls <- conf
		runtime.bus = parent
		return runtime, nil
	}
	r.startController = func(
		context.Context,
		bus.Bus,
		*plugin_space.Config,
	) (*plugin_space.Controller, directive.Reference, error) {
		return nil, ref, nil
	}
	waitTerminal := make(chan struct{})
	waiterDone := make(chan struct{})
	r.waitRuntimeTerminal = func(
		seq uint64,
		runtime *spaceRuntime,
		ctrlRef directive.Reference,
		conf *plugin_space.Config,
	) {
		<-waitTerminal
		r.runRuntimeTerminalWaiter(seq, runtime, ctrlRef, conf)
		close(waiterDone)
	}

	conf := &plugin_space.Config{SpaceId: "space-test"}
	r.StartController(conf)
	if got := <-startCalls; got == conf || got.GetSpaceId() != "space-test" {
		t.Fatalf("startup config = %#v, want owned clone of %#v", got, conf)
	}
	waitSpaceContentsCtrlRef(t, r, ref)

	terminal <- errSpaceRuntimePluginHostSetChanged
	r.Release()
	close(waitTerminal)
	select {
	case <-waiterDone:
	case <-t.Context().Done():
		t.Fatal("host-change terminal waiter did not exit after release")
	}
	waitSpaceContentsRefReleased(t, ref, "resource release")
	assertSpaceContentsCtrlRef(t, r, nil)
	select {
	case extra := <-startCalls:
		t.Fatalf("Release allowed replacement startup: %#v", extra)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestSpaceContentsResourceHostRestartCannotSupersedeNewerStart(t *testing.T) {
	r := NewSpaceContentsResource(nil, nil, nil, "space-test", "engine-test")
	defer r.Release()

	terminal := make(chan error, 1)
	initialRuntime := &spaceRuntime{terminal: terminal}
	startCalls := make(chan *plugin_space.Config, 3)
	var startCount atomic.Int32
	r.startRuntime = func(
		_ context.Context,
		parent bus.Bus,
		_ *logrus.Entry,
		conf *plugin_space.Config,
	) (*spaceRuntime, error) {
		startCalls <- conf
		if startCount.Add(1) == 1 {
			initialRuntime.bus = parent
			return initialRuntime, nil
		}
		return &spaceRuntime{bus: parent, done: make(chan struct{}), terminal: make(chan error)}, nil
	}

	initialRef := newBlockingSpaceContentsRef()
	hostRestartRef := newTestSpaceContentsRef()
	newRef := newTestSpaceContentsRef()
	var controllerCount atomic.Int32
	r.startController = func(
		_ context.Context,
		_ bus.Bus,
		conf *plugin_space.Config,
	) (*plugin_space.Controller, directive.Reference, error) {
		if controllerCount.Add(1) == 1 {
			return nil, initialRef, nil
		}
		if conf.GetSpaceId() == "new-space" {
			return nil, newRef, nil
		}
		return nil, hostRestartRef, nil
	}

	r.StartController(&plugin_space.Config{SpaceId: "old-space"})
	if got := recvSpaceContentsStartConfig(t, startCalls, "initial start"); got.GetSpaceId() != "old-space" {
		t.Fatalf("initial config = %q, want old-space", got.GetSpaceId())
	}
	waitSpaceContentsCtrlRef(t, r, initialRef)

	terminal <- errSpaceRuntimePluginHostSetChanged
	waitBlockingSpaceContentsRefReleaseStarted(t, initialRef)
	if got := recvSpaceContentsStartConfig(t, startCalls, "host restart"); got.GetSpaceId() != "old-space" {
		t.Fatalf("host restart config = %q, want old-space", got.GetSpaceId())
	}

	r.StartController(&plugin_space.Config{SpaceId: "new-space"})
	if got := recvSpaceContentsStartConfig(t, startCalls, "newer start"); got.GetSpaceId() != "new-space" {
		t.Fatalf("newer start config = %q, want new-space", got.GetSpaceId())
	}
	waitSpaceContentsCtrlRef(t, r, newRef)
	waitSpaceContentsRefReleased(t, hostRestartRef, "host restart replacement")

	close(initialRef.unblock)
	waitSpaceContentsRefReleased(t, initialRef.testSpaceContentsRef, "initial controller release")
	assertSpaceContentsCtrlRef(t, r, newRef)
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

type blockingSpaceContentsRef struct {
	*testSpaceContentsRef
	releaseStarted chan struct{}
	unblock        chan struct{}
}

func newBlockingSpaceContentsRef() *blockingSpaceContentsRef {
	return &blockingSpaceContentsRef{
		testSpaceContentsRef: newTestSpaceContentsRef(),
		releaseStarted:       make(chan struct{}),
		unblock:              make(chan struct{}),
	}
}

func (r *blockingSpaceContentsRef) Release() {
	r.once.Do(func() {
		close(r.releaseStarted)
		<-r.unblock
		close(r.released)
	})
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

func recvSpaceContentsStartConfig(
	t *testing.T,
	calls <-chan *plugin_space.Config,
	name string,
) *plugin_space.Config {
	t.Helper()
	select {
	case conf := <-calls:
		return conf
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
		return nil
	}
}

func waitBlockingSpaceContentsRefReleaseStarted(t *testing.T, ref *blockingSpaceContentsRef) {
	t.Helper()
	select {
	case <-ref.releaseStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial controller release")
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

// spaceRuntimePluginHost from runtime_proof_test.go carries the full
// bldr_plugin_host.PluginHost surface; tests here only need platform IDs.

func newHostWatchTest() (*spaceRuntimeHostWatch, *spacePluginHostMirror, <-chan error, <-chan error) {
	mirror := newSpacePluginHostMirror()
	hostReady := make(chan error, 1)
	terminal := make(chan error, 8)
	w := &spaceRuntimeHostWatch{
		mirror:         mirror,
		hostReady:      hostReady,
		reportTerminal: func(err error) { terminal <- err },
	}
	return w, mirror, hostReady, terminal
}

func hostsOf(ids ...string) []bldr_plugin_host.PluginHost {
	out := make([]bldr_plugin_host.PluginHost, 0, len(ids))
	for _, id := range ids {
		out = append(out, &spaceRuntimePluginHost{platformID: id})
	}
	return out
}

func TestSpaceRuntimeHostWatchIdenticalRedeliverySurvives(t *testing.T) {
	w, _, _, _ := newHostWatchTest()
	if err := w.deliver(nil, hostsOf("a", "b")); err != nil {
		t.Fatalf("initial delivery returned %v", err)
	}
	if err := w.deliver(nil, hostsOf("a", "b")); err != nil {
		t.Fatalf("identical redelivery terminated the runtime: %v", err)
	}
}

func TestSpaceRuntimeHostWatchInitialErrorFailsStartupWithoutRuntime(t *testing.T) {
	w, _, hostReady, terminal := newHostWatchTest()
	sent := errors.New("resolver failed")
	if err := w.deliver([]error{sent}, nil); err != nil {
		t.Fatalf("error delivery returned %v", err)
	}
	select {
	case err := <-hostReady:
		if !errors.Is(err, sent) {
			t.Fatalf("startup error = %v, want %v", err, sent)
		}
	case <-time.After(time.Second):
		t.Fatal("startup did not fail with the watch error")
	}
	select {
	case err := <-terminal:
		t.Fatalf("initial error published a terminal: %v", err)
	default:
	}
}

func TestSpaceRuntimeHostWatchPostReadyErrorIsNamedTerminal(t *testing.T) {
	w, _, _, _ := newHostWatchTest()
	if err := w.deliver(nil, hostsOf("a")); err != nil {
		t.Fatalf("initial delivery returned %v", err)
	}
	sent := errors.New("watch broke")
	if err := w.deliver([]error{sent}, nil); err != nil {
		t.Fatalf("error delivery returned %v", err)
	}
}

func TestSpaceRuntimeHostWatchGenuineEmptyCancels(t *testing.T) {
	w, _, _, _ := newHostWatchTest()
	if err := w.deliver(nil, hostsOf("a", "b")); err != nil {
		t.Fatalf("initial delivery returned %v", err)
	}
	err := w.deliver(nil, nil)
	if !errors.Is(err, errSpaceRuntimePluginHostSetChanged) {
		t.Fatalf("genuine empty delivery = %v, want %v", err, errSpaceRuntimePluginHostSetChanged)
	}
}

func TestSpaceRuntimeHostWatchMembershipChangeCancels(t *testing.T) {
	w, _, _, _ := newHostWatchTest()
	if err := w.deliver(nil, hostsOf("a", "b")); err != nil {
		t.Fatalf("initial delivery returned %v", err)
	}
	if err := w.deliver(nil, hostsOf("a")); !errors.Is(err, errSpaceRuntimePluginHostSetChanged) {
		t.Fatalf("membership loss = %v, want %v", err, errSpaceRuntimePluginHostSetChanged)
	}
}

func TestSpaceRuntimeHostWatchDuplicateGrowthCancels(t *testing.T) {
	w, _, _, _ := newHostWatchTest()
	if err := w.deliver(nil, hostsOf("a")); err != nil {
		t.Fatalf("initial delivery returned %v", err)
	}
	if err := w.deliver(nil, hostsOf("a", "a")); !errors.Is(err, errSpaceRuntimePluginHostSetChanged) {
		t.Fatalf("duplicate growth = %v, want %v", err, errSpaceRuntimePluginHostSetChanged)
	}
}
