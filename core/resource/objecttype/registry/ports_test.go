package resource_objecttype_registry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	"github.com/sirupsen/logrus"
)

// TestRegisteredObjectTypeDeclaresPorts registers a type with two ports through
// the registry resource, reads the ports back through the bus lookup and
// WatchObjectTypes, and checks the lookup is empty once the registration is
// released.
func TestRegisteredObjectTypeDeclaresPorts(t *testing.T) {
	// Start the World testbed whose bus hosts the bridge.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Start the registry, its bridge controller, and a caller resource client.
	registry, resources, registryClient := newRegistryResourceClient(t, ctx)
	defer resources.Release()
	rel, err := tb.Bus.AddController(ctx, NewBridgeController(le, tb.Bus, registry), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// Nothing is registered yet, so the lookup is empty.
	if err := expectMetadataPorts(t, tb.Bus, "test/ported", nil); err != nil {
		t.Fatalf("lookup before registration: %v", err)
	}

	// Register a type that declares one input and one output port.
	ports := []*s4wave_objecttype_registry.ObjectTypePort{
		{Name: "in", Direction: s4wave_objecttype_registry.ObjectTypePortDirection_OBJECT_TYPE_PORT_DIRECTION_INPUT, TypeId: "test/stream"},
		{Name: "out", Direction: s4wave_objecttype_registry.ObjectTypePortDirection_OBJECT_TYPE_PORT_DIRECTION_OUTPUT, TypeId: "test/event"},
	}
	registration, err := registryClient.RegisterObjectType(ctx, &s4wave_objecttype_registry.RegisterObjectTypeRequest{
		TypeId:   "test/ported",
		PluginId: "test-plugin",
		Metadata: &s4wave_objecttype_registry.ObjectTypeMetadata{DisplayName: "Ported", Ports: ports},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Read the ports back through the bus lookup.
	if err := expectMetadataPorts(t, tb.Bus, "test/ported", ports); err != nil {
		t.Fatalf("lookup after registration: %v", err)
	}
	other, err := s4wave_objecttype_registry.ExLookupObjectTypeMetadata(ctx, tb.Bus, "test/other")
	if err != nil || other != nil {
		t.Fatalf("lookup of an unregistered type = %v, %v; want nil", other, err)
	}

	// A mutated lookup result does not change the stored registration.
	got, err := s4wave_objecttype_registry.ExLookupObjectTypeMetadata(ctx, tb.Bus, "test/ported")
	if err != nil || got == nil {
		t.Fatalf("lookup: %v, %v", got, err)
	}
	got.Ports[0].Name = "mutated"
	if err := expectMetadataPorts(t, tb.Bus, "test/ported", ports); err != nil {
		t.Fatalf("lookup after mutating a result: %v", err)
	}

	// Read the same ports from the first WatchObjectTypes message.
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	strm, err := registryClient.WatchObjectTypes(watchCtx, &s4wave_objecttype_registry.WatchObjectTypesRequest{})
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	msg, err := strm.Recv()
	if err != nil {
		t.Fatalf("watch recv: %v", err)
	}

	// The message carries the registration's ports and the host's built-in types.
	if len(msg.GetRegistrations()) != 1 {
		t.Fatalf("watched registrations = %d, want 1", len(msg.GetRegistrations()))
	}
	if err := comparePorts(msg.GetRegistrations()[0].GetMetadata().GetPorts(), ports); err != nil {
		t.Fatalf("watched ports: %v", err)
	}
	if !slices.Equal(msg.GetBuiltInTypeIds(), testBuiltInTypeIDs) {
		t.Fatalf("watched built-in types = %v", msg.GetBuiltInTypeIds())
	}

	// Release the registration resource and wait for the watch to show it gone.
	resources.CreateResourceReference(registration.GetResourceId()).Release()
	msg, err = strm.Recv()
	if err != nil {
		t.Fatalf("watch recv after release: %v", err)
	}
	if len(msg.GetRegistrations()) != 0 {
		t.Fatalf("watched registrations after release = %d, want 0", len(msg.GetRegistrations()))
	}
	if err := expectMetadataPorts(t, tb.Bus, "test/ported", nil); err != nil {
		t.Fatalf("lookup after release: %v", err)
	}
}

// TestWatchObjectTypeMetadata watches one type's metadata and sees a
// registration, its release and a later registration, and nothing after the
// watch is released.
func TestWatchObjectTypeMetadata(t *testing.T) {
	// Start the registry and its bridge controller.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	registry, resources, registryClient := newRegistryResourceClient(t, ctx)
	defer resources.Release()
	rel, err := tb.Bus.AddController(ctx, NewBridgeController(le, tb.Bus, registry), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// Watch a type nothing registers: a registration of another type does not
	// reach the callback.
	changes := make(chan *s4wave_objecttype_registry.ObjectTypeMetadata, 8)
	release, err := s4wave_objecttype_registry.ExWatchObjectTypeMetadata(ctx, tb.Bus, "test/watched", "", func(metadata *s4wave_objecttype_registry.ObjectTypeMetadata) {
		changes <- metadata
	})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	register := func(typeID, name string) *s4wave_objecttype_registry.RegisterObjectTypeResponse {
		registration, err := registryClient.RegisterObjectType(ctx, &s4wave_objecttype_registry.RegisterObjectTypeRequest{
			TypeId:   typeID,
			PluginId: "test-plugin",
			Metadata: &s4wave_objecttype_registry.ObjectTypeMetadata{DisplayName: name},
		})
		if err != nil {
			t.Fatalf("register %s: %v", typeID, err)
		}
		return registration
	}
	expect := func(want string) {
		t.Helper()
		select {
		case got := <-changes:
			if want == "" && got != nil || want != "" && got.GetDisplayName() != want {
				t.Fatalf("metadata = %v, want %q", got, want)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("no change, want %q", want)
		}
	}
	other := register("test/other", "Other")
	defer resources.CreateResourceReference(other.GetResourceId()).Release()
	select {
	case got := <-changes:
		t.Fatalf("a registration of another type changed the watch: %v", got)
	case <-time.After(100 * time.Millisecond):
	}

	// Registering the watched type reaches the callback, and releasing it sends nil.
	first := register("test/watched", "First")
	expect("First")
	resources.CreateResourceReference(first.GetResourceId()).Release()
	expect("")

	// Registering the watched type again reaches the callback, and a released
	// watch hears nothing more.
	second := register("test/watched", "Second")
	expect("Second")
	release()
	resources.CreateResourceReference(second.GetResourceId()).Release()
	select {
	case got := <-changes:
		t.Fatalf("a released watch changed: %v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

// expectMetadataPorts checks the bus lookup returns want, or nothing when want is nil.
func expectMetadataPorts(t *testing.T, b bus.Bus, typeID string, want []*s4wave_objecttype_registry.ObjectTypePort) error {
	t.Helper()
	got, err := s4wave_objecttype_registry.ExLookupObjectTypeMetadata(t.Context(), b, typeID)
	if err != nil {
		return err
	}
	if want == nil {
		if got != nil {
			return fmt.Errorf("metadata = %v, want none", got)
		}
		return nil
	}
	if got == nil {
		return errors.New("metadata is nil, want registered metadata")
	}
	return comparePorts(got.GetPorts(), want)
}

// comparePorts reports the first difference between two port lists.
func comparePorts(got, want []*s4wave_objecttype_registry.ObjectTypePort) error {
	if len(got) != len(want) {
		return fmt.Errorf("ports = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if !got[i].EqualVT(want[i]) {
			return fmt.Errorf("port %d = %v, want %v", i, got[i], want[i])
		}
	}
	return nil
}
