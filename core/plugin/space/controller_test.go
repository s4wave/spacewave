package plugin_space

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/keyed"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestReconcileProcessConfigsClearsWhenStoreUnavailable(t *testing.T) {
	c := &Controller{
		processConfigs: map[string]processConfig{
			"stale-process": {typeID: "test/process"},
		},
	}
	c.processes = keyed.NewKeyed(func(key string) (keyed.Routine, processConfig) {
		return nil, c.processConfigs[key]
	})
	c.processes.SyncKeys([]string{"stale-process"}, false)

	c.reconcileProcessConfigs(logrus.NewEntry(logrus.New()), nil)

	if got := c.processes.GetKeys(); len(got) != 0 {
		t.Fatalf("process keys after unavailable store = %v, want none", got)
	}
	if got := len(c.processConfigs); got != 0 {
		t.Fatalf("process config count after unavailable store = %d, want 0", got)
	}
}

func TestProcessResolversTreatsEmptySpaceManifestLookupAsCacheMiss(t *testing.T) {
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	handler := &recordResolverHandler{}
	entry := &resolverEntry{
		ctx:     ctx,
		dir:     manifest.NewFetchManifest("spacewave-notes", nil, []string{"js", "web/js/wasm"}, 0),
		handler: handler,
	}
	c := newTestResolverController()
	c.pluginIDs = []string{"spacewave-notes"}
	c.resolvers[entry] = struct{}{}

	c.processResolvers(ctx, tb.WorldState)

	if handler.added != 0 {
		t.Fatalf("empty manifest lookup added %d values, want 0", handler.added)
	}
	if handler.cleared != 0 {
		t.Fatalf("empty manifest lookup cleared %d times with no previous value, want 0", handler.cleared)
	}
	if !handler.idle {
		t.Fatal("resolver was not marked idle")
	}
}

func TestProcessResolversClearsPreviousValueOnEmptySpaceManifestLookup(t *testing.T) {
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	handler := &recordResolverHandler{}
	entry := &resolverEntry{
		ctx:     ctx,
		dir:     manifest.NewFetchManifest("spacewave-notes", nil, []string{"js", "web/js/wasm"}, 0),
		handler: handler,
		emitted: manifest.NewFetchManifestValue([]*manifest.ManifestRef{{
			Meta: &manifest.ManifestMeta{
				ManifestId: "spacewave-notes",
				PlatformId: "js",
				Rev:        1,
			},
		}}),
	}
	c := newTestResolverController()
	c.pluginIDs = []string{"spacewave-notes"}
	c.resolvers[entry] = struct{}{}

	c.processResolvers(ctx, tb.WorldState)

	if handler.added != 0 {
		t.Fatalf("empty manifest lookup added %d values, want 0", handler.added)
	}
	if handler.cleared != 1 {
		t.Fatalf("empty manifest lookup cleared %d times, want 1", handler.cleared)
	}
	if entry.emitted != nil {
		t.Fatalf("entry.emitted = %#v, want nil", entry.emitted)
	}
	if !handler.idle {
		t.Fatal("resolver was not marked idle")
	}
}

func newTestResolverController() *Controller {
	return &Controller{
		BusController: bus.NewBusController(
			logrus.NewEntry(logrus.New()),
			nil,
			&Config{},
			ControllerID,
			Version,
			controllerDescrip,
		),
		resolvers: make(map[*resolverEntry]struct{}),
	}
}

type recordResolverHandler struct {
	values  []directive.Value
	nextID  uint32
	added   int
	cleared int
	idle    bool
}

func (h *recordResolverHandler) AddValue(value directive.Value) (uint32, bool) {
	h.nextID++
	h.added++
	h.values = append(h.values, value)
	return h.nextID, true
}

func (h *recordResolverHandler) RemoveValue(id uint32) (directive.Value, bool) {
	if id == 0 || int(id) > len(h.values) {
		return nil, false
	}
	value := h.values[id-1]
	h.values[id-1] = nil
	return value, true
}

func (h *recordResolverHandler) CountValues(_ bool) int {
	return len(h.values)
}

func (h *recordResolverHandler) ClearValues() []uint32 {
	removed := make([]uint32, len(h.values))
	for idx := range h.values {
		removed[idx] = uint32(idx + 1)
	}
	h.values = nil
	h.cleared++
	return removed
}

func (h *recordResolverHandler) MarkIdle(idle bool) {
	h.idle = idle
}

func (h *recordResolverHandler) AddValueRemovedCallback(uint32, func()) func() {
	return func() {}
}

func (h *recordResolverHandler) AddResolverRemovedCallback(func()) func() {
	return func() {}
}

func (h *recordResolverHandler) AddResolver(directive.Resolver, func()) func() {
	return func() {}
}
