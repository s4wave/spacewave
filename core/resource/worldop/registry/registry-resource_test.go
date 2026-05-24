package resource_worldop_registry

import (
	"testing"

	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
)

// TestNewWorldOpRegistryResource tests basic construction.
func TestNewWorldOpRegistryResource(t *testing.T) {
	r := NewWorldOpRegistryResource()
	if r == nil {
		t.Fatal("expected non-nil resource")
	}
	if r.GetMux() == nil {
		t.Fatal("expected non-nil mux")
	}
	if r.registrations == nil {
		t.Fatal("expected non-nil registrations map")
	}
	if r.nextID != 1 {
		t.Fatalf("expected nextID=1, got %d", r.nextID)
	}
}

// TestLookupRegistrationByOpTypeEmpty tests that LookupRegistrationByOpType returns nil for unknown ops.
func TestLookupRegistrationByOpTypeEmpty(t *testing.T) {
	r := NewWorldOpRegistryResource()
	reg := r.LookupRegistrationByOpType("unknown/op")
	if reg != nil {
		t.Fatal("expected nil for unknown operation type")
	}
}

// TestLookupRegistrationByOpTypeFound tests that LookupRegistrationByOpType finds a manually added registration.
func TestLookupRegistrationByOpTypeFound(t *testing.T) {
	r := NewWorldOpRegistryResource()

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_worldop_registry.WorldOpRegistration{
			OperationTypeId: "test-plugin/test-op",
			RegistrationId:  1,
			PluginId:        "test-plugin",
		}
		broadcast()
	})

	reg := r.LookupRegistrationByOpType("test-plugin/test-op")
	if reg == nil {
		t.Fatal("expected non-nil registration")
	}
	if reg.GetOperationTypeId() != "test-plugin/test-op" {
		t.Fatalf("expected operation_type_id test-plugin/test-op, got %s", reg.GetOperationTypeId())
	}
	if reg.GetRegistrationId() != 1 {
		t.Fatalf("expected registration_id 1, got %d", reg.GetRegistrationId())
	}
	if reg.GetPluginId() != "test-plugin" {
		t.Fatalf("expected plugin_id test-plugin, got %s", reg.GetPluginId())
	}
}

// TestLookupRegistrationByOpTypeReturnsClone tests that the returned registration is a clone.
func TestLookupRegistrationByOpTypeReturnsClone(t *testing.T) {
	r := NewWorldOpRegistryResource()

	orig := &s4wave_worldop_registry.WorldOpRegistration{
		OperationTypeId: "test-plugin/cloned",
		RegistrationId:  1,
		PluginId:        "test-plugin",
	}
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = orig
		broadcast()
	})

	reg := r.LookupRegistrationByOpType("test-plugin/cloned")
	if reg == nil {
		t.Fatal("expected non-nil registration")
	}

	// Mutating the returned value should not affect the stored one.
	reg.OperationTypeId = "mutated"
	reg2 := r.LookupRegistrationByOpType("test-plugin/cloned")
	if reg2 == nil {
		t.Fatal("expected registration to still exist after mutating clone")
	}
	if reg2.GetOperationTypeId() != "test-plugin/cloned" {
		t.Fatalf("stored registration was mutated: got %s", reg2.GetOperationTypeId())
	}
}

// TestLookupRegistrationByOpTypeMultiple tests lookup with multiple registrations.
func TestLookupRegistrationByOpTypeMultiple(t *testing.T) {
	r := NewWorldOpRegistryResource()

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_worldop_registry.WorldOpRegistration{
			OperationTypeId: "plugin-a/op-one",
			RegistrationId:  1,
			PluginId:        "plugin-a",
		}
		r.registrations[2] = &s4wave_worldop_registry.WorldOpRegistration{
			OperationTypeId: "plugin-b/op-two",
			RegistrationId:  2,
			PluginId:        "plugin-b",
		}
		r.registrations[3] = &s4wave_worldop_registry.WorldOpRegistration{
			OperationTypeId: "plugin-a/op-three",
			RegistrationId:  3,
			PluginId:        "plugin-a",
		}
		broadcast()
	})

	reg := r.LookupRegistrationByOpType("plugin-b/op-two")
	if reg == nil {
		t.Fatal("expected to find plugin-b/op-two")
	}
	if reg.GetRegistrationId() != 2 {
		t.Fatalf("expected registration_id 2, got %d", reg.GetRegistrationId())
	}

	reg = r.LookupRegistrationByOpType("plugin-a/op-three")
	if reg == nil {
		t.Fatal("expected to find plugin-a/op-three")
	}
	if reg.GetRegistrationId() != 3 {
		t.Fatalf("expected registration_id 3, got %d", reg.GetRegistrationId())
	}

	reg = r.LookupRegistrationByOpType("nonexistent/op")
	if reg != nil {
		t.Fatal("expected nil for nonexistent operation type")
	}
}

// TestGetRegistrationsLocked tests the snapshot helper.
func TestGetRegistrationsLocked(t *testing.T) {
	r := NewWorldOpRegistryResource()

	// Empty registry should return empty slice.
	var regs []*s4wave_worldop_registry.WorldOpRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		regs = r.getRegistrationsLocked()
	})
	if len(regs) != 0 {
		t.Fatalf("expected 0 registrations, got %d", len(regs))
	}

	// Add two registrations.
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_worldop_registry.WorldOpRegistration{
			OperationTypeId: "p/a",
			RegistrationId:  1,
			PluginId:        "p",
		}
		r.registrations[2] = &s4wave_worldop_registry.WorldOpRegistration{
			OperationTypeId: "p/b",
			RegistrationId:  2,
			PluginId:        "p",
		}
		broadcast()
	})

	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		regs = r.getRegistrationsLocked()
	})
	if len(regs) != 2 {
		t.Fatalf("expected 2 registrations, got %d", len(regs))
	}
}

// TestRegistrationRemoval tests that deleting a registration makes it unfindable.
func TestRegistrationRemoval(t *testing.T) {
	r := NewWorldOpRegistryResource()

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_worldop_registry.WorldOpRegistration{
			OperationTypeId: "test-plugin/removable",
			RegistrationId:  1,
			PluginId:        "test-plugin",
		}
		broadcast()
	})

	reg := r.LookupRegistrationByOpType("test-plugin/removable")
	if reg == nil {
		t.Fatal("expected registration before removal")
	}

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(r.registrations, 1)
		broadcast()
	})

	reg = r.LookupRegistrationByOpType("test-plugin/removable")
	if reg != nil {
		t.Fatal("expected nil after removal")
	}
}

// TestBroadcastOnChange tests that the broadcast channel fires when registrations change.
func TestBroadcastOnChange(t *testing.T) {
	r := NewWorldOpRegistryResource()

	var waitCh <-chan struct{}
	r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
	})

	// Channel should not be closed yet.
	select {
	case <-waitCh:
		t.Fatal("wait channel closed before any change")
	default:
	}

	// Add a registration with broadcast.
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_worldop_registry.WorldOpRegistration{
			OperationTypeId: "test-plugin/broadcast",
			RegistrationId:  1,
			PluginId:        "test-plugin",
		}
		broadcast()
	})

	// Channel should now be closed.
	select {
	case <-waitCh:
	default:
		t.Fatal("wait channel not closed after broadcast")
	}
}
