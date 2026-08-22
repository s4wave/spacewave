package plugin_host_scheduler

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"testing"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/sirupsen/logrus"
)

func TestPluginInstanceWaitsForInitialCapabilityRegistration(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainer(&PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*bldr_plugin.PluginStatus),
	}
	instance := &pluginInstance{
		c:                ctrl,
		le:               le,
		pluginID:         "test-plugin",
		runningPluginCtr: ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
		pluginLoadStateCtr: ccontainer.NewCContainer(
			bldr_plugin.NewPluginLoadState(
				nil,
				bldr_plugin.InitialCapabilityRegistrationPending,
			),
		),
	}

	instance.beginInitialCapabilityRegistration()
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewMux())))
	instance.updateRpcClient(client)
	if running := instance.runningPluginCtr.GetValue(); running != nil {
		t.Fatal("RPC-connected plugin reported running before initial registration")
	}
	state := instance.pluginLoadStateCtr.GetValue()
	if state.GetRpcClient() == nil {
		t.Fatal("expected RPC client during initial registration")
	}
	if state.GetInitialCapabilityRegistrationState() != bldr_plugin.InitialCapabilityRegistrationPending {
		t.Fatalf("registration state = %v, want pending", state.GetInitialCapabilityRegistrationState())
	}

	instance.finishInitialCapabilityRegistration(true)
	if running := instance.runningPluginCtr.GetValue(); running == nil {
		t.Fatal("plugin did not report running after initial registration")
	}
	state = instance.pluginLoadStateCtr.GetValue()
	if state.GetInitialCapabilityRegistrationState() != bldr_plugin.InitialCapabilityRegistrationComplete {
		t.Fatalf("registration state = %v, want complete", state.GetInitialCapabilityRegistrationState())
	}
}

// loadPluginValuesHandler collects RunningPlugin values emitted by the
// LoadPlugin resolver.
type loadPluginValuesHandler struct {
	mtx    sync.Mutex
	nextID uint32
	// rev increments on every handler event so waiters can distinguish the
	// step that triggered them from a coalesced earlier wake.
	rev    uint64
	values map[uint32]bldr_plugin.RunningPlugin
	idle   bool
	// changed wakes the test waiter after any state mutation.
	changed chan struct{}
}

func newLoadPluginValuesHandler() *loadPluginValuesHandler {
	return &loadPluginValuesHandler{
		values:  make(map[uint32]bldr_plugin.RunningPlugin),
		changed: make(chan struct{}, 1),
	}
}

// notify records that handler state changed without blocking.
// The caller must hold h.mtx so rev and the value map stay consistent.
func (h *loadPluginValuesHandler) notify() {
	h.rev++
	select {
	case h.changed <- struct{}{}:
	default:
	}
}

func (h *loadPluginValuesHandler) AddValue(v directive.Value) (uint32, bool) {
	running, ok := v.(bldr_plugin.RunningPlugin)
	if !ok {
		return 0, false
	}
	h.mtx.Lock()
	defer h.mtx.Unlock()
	h.nextID++
	h.values[h.nextID] = running
	h.notify()
	return h.nextID, true
}

func (h *loadPluginValuesHandler) RemoveValue(id uint32) (directive.Value, bool) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	v, found := h.values[id]
	delete(h.values, id)
	if !found {
		return nil, false
	}
	h.notify()
	return v, true
}

func (h *loadPluginValuesHandler) CountValues(allResolvers bool) int {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return len(h.values)
}

func (h *loadPluginValuesHandler) ClearValues() []uint32 {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	ids := make([]uint32, 0, len(h.values))
	for id := range h.values {
		ids = append(ids, id)
	}
	h.values = make(map[uint32]bldr_plugin.RunningPlugin)
	slices.Sort(ids)
	if len(ids) != 0 {
		h.notify()
	}
	return ids
}

// MarkIdle always notifies: the test steps key off handler events, not the
// idle boolean, so even an unchanged value must wake the waiter.
func (h *loadPluginValuesHandler) MarkIdle(idle bool) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	h.idle = idle
	h.notify()
}

func (h *loadPluginValuesHandler) AddValueRemovedCallback(uint32, func()) func() {
	return func() {}
}

func (h *loadPluginValuesHandler) AddResolver(directive.Resolver, func()) func() {
	return func() {}
}

func (h *loadPluginValuesHandler) AddResolverRemovedCallback(func()) func() {
	return func() {}
}

// handlerSnapshot captures the values, idle flag, and revision the handler
// has published so far.
type handlerSnapshot struct {
	vals []bldr_plugin.RunningPlugin
	idle bool
	rev  uint64
}

func (h *loadPluginValuesHandler) snapshot() handlerSnapshot {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return handlerSnapshot{
		vals: slices.Collect(maps.Values(h.values)),
		idle: h.idle,
		rev:  h.rev,
	}
}

func (h *loadPluginValuesHandler) revision() uint64 {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.rev
}

// waitChange blocks until the handler revision passes rev, re-checking after
// each coalesced wake or context cancellation with no timer. It then returns
// the immediate post-change snapshot.
func waitChange(t *testing.T, h *loadPluginValuesHandler, rev uint64) handlerSnapshot {
	t.Helper()
	for {
		snap := h.snapshot()
		if snap.rev > rev {
			return snap
		}
		select {
		case <-h.changed:
		case <-t.Context().Done():
			t.Fatalf("handler did not change after revision %d: %v", rev, t.Context().Err())
		}
	}
}

// TestLoadPluginResolverWaitsForWorkerRpcConnection resolves the LoadPlugin
// directive through the host resolver and proves a RunningPlugin value is
// withheld until the worker RPC connection and initial capability
// registration complete.
func TestLoadPluginResolverWaitsForWorkerRpcConnection(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(t.Context())
	defer ctxCancel()
	le := logrus.NewEntry(logrus.New())
	ctrl := &Controller{
		le:              le,
		conf:            &Config{},
		pluginStatusCtr: ccontainer.NewCContainer(&PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*bldr_plugin.PluginStatus),
	}
	ctrl.pluginInstances = keyed.NewKeyedRefCountWithLogger(ctrl.newPluginInstance, le)

	_, relRef := ctrl.AddPluginReference("test-plugin", "")
	defer relRef()
	instance, ok := ctrl.pluginInstances.GetKey(pluginInstanceKey("test-plugin", ""))
	if !ok {
		t.Fatal("plugin instance was not created for LoadPlugin reference")
	}

	handler := newLoadPluginValuesHandler()
	resolver := bldr_plugin_host.NewLoadPluginResolver(ctrl, "test-plugin", "")
	resolveDone := make(chan error, 1)
	go func() { resolveDone <- resolver.Resolve(ctx, handler) }()

	if snap := handler.snapshot(); len(snap.vals) != 0 {
		t.Fatalf("RunningPlugin published before worker connection: %d values", len(snap.vals))
	}
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewMux())))

	// Worker RPC connected but registration is still in flight: no value yet.
	base := handler.revision()
	instance.updateRpcClient(client)
	if snap := waitChange(t, handler, base); len(snap.vals) != 0 {
		t.Fatal("RPC-connected plugin published RunningPlugin before initial registration")
	}

	base = handler.revision()
	instance.finishInitialCapabilityRegistration(true)
	snap := waitChange(t, handler, base)
	if len(snap.vals) != 1 || snap.vals[0].GetRpcClient() != client {
		t.Fatal("published RunningPlugin does not carry the worker RPC client")
	}

	// Worker disconnect clears the published value and marks the resolver idle.
	base = handler.revision()
	instance.updateRpcClient(nil)
	snap = waitChange(t, handler, base)
	if len(snap.vals) != 0 || !snap.idle {
		t.Fatal("worker disconnect did not clear RunningPlugin and mark the resolver idle")
	}

	ctxCancel()
	if err := <-resolveDone; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("resolver returned %v, want context canceled", err)
	}
}
