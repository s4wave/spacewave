package bldr_plugin_admit

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/callback"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// ownerID is the plugin that owns the approvals in these tests.
const ownerID = "owner"

// testOwner is a fake approvals owner: a plugin on the host bus that runs
// while running is set and serves PluginRootAdmission from its own bus, where
// admitted decides the root.
type testOwner struct {
	bcast    broadcast.Broadcast
	running  bool
	admitted bool
	// breaks counts stream breaks to inject: each open stream ends with an
	// error when breaks rises.
	breaks int
	// hold keeps a new stream from answering while set.
	hold bool
}

// set changes the owner's state under its lock and wakes its watchers.
func (o *testOwner) set(fn func()) {
	l := o.bcast.Lock()
	fn()
	l.Broadcast()
	l.Unlock()
}

// breakingServer ends each stream with an error when the owner injects a break.
type breakingServer struct {
	*ViaBus
	o *testOwner
}

// WatchPluginRootAdmitted serves the owner's answers until a break, once the
// owner stops holding new streams.
func (s *breakingServer) WatchPluginRootAdmitted(req *WatchPluginRootAdmittedRequest, strm SRPCPluginRootAdmission_WatchPluginRootAdmittedStream) error {
	var breaks int
	var waitCh <-chan struct{}
	for {
		l := s.o.bcast.Lock()
		hold := s.o.hold
		breaks, waitCh = s.o.breaks, l.WaitCh()
		l.Unlock()
		if !hold {
			break
		}
		select {
		case <-strm.Context().Done():
			return strm.Context().Err()
		case <-waitCh:
		}
	}
	ctx, cancel := context.WithCancel(strm.Context())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- s.ViaBus.WatchPluginRootAdmitted(req, &ctxStream{strm, ctx}) }()
	for {
		select {
		case err := <-errCh:
			return err
		case <-waitCh:
		}
		l := s.o.bcast.Lock()
		broke := s.o.breaks != breaks
		waitCh = l.WaitCh()
		l.Unlock()
		if broke {
			return context.DeadlineExceeded
		}
	}
}

// ctxStream overrides a stream's context.
type ctxStream struct {
	SRPCPluginRootAdmission_WatchPluginRootAdmittedStream
	ctx context.Context
}

// Context returns the overridden context.
func (s *ctxStream) Context() context.Context { return s.ctx }

// testRunningPlugin is a running owner reached through client.
type testRunningPlugin struct{ client srpc.Client }

// GetRpcClient returns the owner's client.
func (p testRunningPlugin) GetRpcClient() srpc.Client { return p.client }

// newHost builds a host bus with the admit controller for ownerID, and an
// owner whose bus answers AdmitPluginRoot from its admitted flag.
func newHost(ctx context.Context, t *testing.T) (bus.Bus, *testOwner) {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	host, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Release)
	ownerBus, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ownerBus.Release)
	o := &testOwner{}

	// The owner's bus admits the root while admitted is set, and goes idle.
	add := func(b bus.Bus, name string, fn func(directive.Instance) func(context.Context, directive.ResolverHandler) error) {
		ctrl := callback.NewCallbackController(
			controller.NewInfo(name, controller.MustParseVersion("0.0.1"), ""),
			nil,
			func(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
				if res := fn(inst); res != nil {
					return directive.R(directive.NewFuncResolver(res), nil)
				}
				return nil, nil
			},
			nil,
		)
		rel, err := b.AddController(ctx, ctrl, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(rel)
	}
	// follow holds a value while get reports true, and idles in each state.
	follow := func(get func() bool, val directive.Value) func(context.Context, directive.ResolverHandler) error {
		return func(ctx context.Context, handler directive.ResolverHandler) error {
			var id uint32
			for {
				l := o.bcast.Lock()
				on, waitCh := get(), l.WaitCh()
				l.Unlock()
				switch {
				case on && id == 0:
					id, _ = handler.AddValue(val)
				case !on && id != 0:
					_, _ = handler.RemoveValue(id)
					id = 0
				}
				handler.MarkIdle(true)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-waitCh:
				}
			}
		}
	}
	add(ownerBus.Bus, "test/approvals", func(inst directive.Instance) func(context.Context, directive.ResolverHandler) error {
		if _, ok := inst.GetDirective().(bldr_plugin.AdmitPluginRoot); !ok {
			return nil
		}
		return follow(func() bool { return o.admitted }, bldr_plugin.AdmitSourceApproved)
	})

	// The host runs the owner while running is set.
	mux := srpc.NewMux()
	if err := SRPCRegisterPluginRootAdmission(mux, &breakingServer{ViaBus: NewViaBus(ownerBus.Bus), o: o}); err != nil {
		t.Fatal(err)
	}
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
	add(host.Bus, "test/owner", func(inst directive.Instance) func(context.Context, directive.ResolverHandler) error {
		d, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
		if !ok || d.LoadPluginID() != ownerID {
			return nil
		}
		return follow(func() bool { return o.running }, bldr_plugin.RunningPlugin(testRunningPlugin{client}))
	})

	// The admit controller under test.
	rel, err := host.Bus.AddController(ctx, NewController(le, host.Bus, &Config{PluginId: ownerID}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rel)
	return host.Bus, o
}

// admission is the observed state of one AdmitPluginRoot on the host bus.
type admission struct {
	bcast  broadcast.Broadcast
	values int
	idle   bool
}

// watchAdmission adds the root's directive on b and tracks its state.
func watchAdmission(ctx context.Context, t *testing.T, b bus.Bus) *admission {
	t.Helper()
	a := &admission{}
	update := func(fn func()) {
		l := a.bcast.Lock()
		fn()
		l.Broadcast()
		l.Unlock()
	}
	di, ref, err := b.AddDirective(
		ctx,
		bldr_plugin.NewAdmitPluginRoot("peer", "colors", testRoot(t)),
		bus.NewCallbackHandler(
			func(directive.AttachedValue) { update(func() { a.values++ }) },
			func(directive.AttachedValue) { update(func() { a.values-- }) },
			nil,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)
	t.Cleanup(di.AddIdleCallback(func(isIdle bool, _ []error) { update(func() { a.idle = isIdle }) }))
	return a
}

// await waits until the admission reaches the wanted values and idle state.
func (a *admission) await(ctx context.Context, t *testing.T, what string, values int, idle bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		l := a.bcast.Lock()
		got, gotIdle, waitCh := a.values, a.idle, l.WaitCh()
		l.Unlock()
		if got == values && gotIdle == idle {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("never reached %s: values %d idle %v", what, got, gotIdle)
		case <-waitCh:
		}
	}
}

// testRoot returns a valid manifest root key.
func testRoot(t *testing.T) string {
	t.Helper()
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("colors"))
	if err != nil {
		t.Fatal(err)
	}
	return h.MarshalString()
}

// TestAdmitAsksOwner checks that the host admits a root while the owner says
// it is admitted, refuses it when the owner says it is not, and asks again
// with no answer whenever the owner exits or its stream breaks.
func TestAdmitAsksOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	b, o := newHost(ctx, t)
	a := watchAdmission(ctx, t, b)

	// With the owner not running, the ask stays unanswered.
	select {
	case <-time.After(200 * time.Millisecond):
	case <-ctx.Done():
	}
	a.await(ctx, t, "unanswered while the owner is not running", 0, false)

	// The owner starts and refuses, then admits.
	o.set(func() { o.running = true })
	a.await(ctx, t, "refused", 0, true)
	o.set(func() { o.admitted = true })
	a.await(ctx, t, "admitted", 1, true)

	// The owner exits: the value goes and the ask has no answer.
	o.set(func() { o.running = false })
	a.await(ctx, t, "unanswered after the owner exits", 0, false)
	o.set(func() { o.running = true })
	a.await(ctx, t, "admitted after the owner returns", 1, true)

	// The stream breaks: the value goes until the retry answers again.
	o.set(func() { o.breaks++; o.hold = true })
	a.await(ctx, t, "unanswered after the stream breaks", 0, false)
	o.set(func() { o.hold = false })
	a.await(ctx, t, "admitted after the retry answers", 1, true)
}
