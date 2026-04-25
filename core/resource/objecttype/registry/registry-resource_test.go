package resource_objecttype_registry

import (
	"testing"

	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
)

// TestNewObjectTypeRegistryResource tests basic construction.
func TestNewObjectTypeRegistryResource(t *testing.T) {
	r := NewObjectTypeRegistryResource()
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

// TestLookupRegistrationEmpty tests that LookupRegistration returns nil for unknown types.
func TestLookupRegistrationEmpty(t *testing.T) {
	r := NewObjectTypeRegistryResource()
	reg := r.LookupRegistration("unknown/type")
	if reg != nil {
		t.Fatal("expected nil for unknown type")
	}
}

// TestLookupRegistrationFound tests that LookupRegistration finds a manually added registration.
func TestLookupRegistrationFound(t *testing.T) {
	r := NewObjectTypeRegistryResource()

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "test-plugin/test-type",
			RegistrationId: 1,
			PluginId:       "test-plugin",
		}
		broadcast()
	})

	reg := r.LookupRegistration("test-plugin/test-type")
	if reg == nil {
		t.Fatal("expected non-nil registration")
	}
	if reg.GetTypeId() != "test-plugin/test-type" {
		t.Fatalf("expected type_id test-plugin/test-type, got %s", reg.GetTypeId())
	}
	if reg.GetRegistrationId() != 1 {
		t.Fatalf("expected registration_id 1, got %d", reg.GetRegistrationId())
	}
	if reg.GetPluginId() != "test-plugin" {
		t.Fatalf("expected plugin_id test-plugin, got %s", reg.GetPluginId())
	}
}

// TestLookupRegistrationReturnsClone tests that LookupRegistration returns a clone.
func TestLookupRegistrationReturnsClone(t *testing.T) {
	r := NewObjectTypeRegistryResource()

	orig := &s4wave_objecttype_registry.ObjectTypeRegistration{
		TypeId:         "test-plugin/cloned",
		RegistrationId: 1,
		PluginId:       "test-plugin",
	}
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = orig
		broadcast()
	})

	reg := r.LookupRegistration("test-plugin/cloned")
	if reg == nil {
		t.Fatal("expected non-nil registration")
	}

	// Mutating the returned value should not affect the stored one.
	reg.TypeId = "mutated"
	reg2 := r.LookupRegistration("test-plugin/cloned")
	if reg2 == nil {
		t.Fatal("expected registration to still exist after mutating clone")
	}
	if reg2.GetTypeId() != "test-plugin/cloned" {
		t.Fatalf("stored registration was mutated: got %s", reg2.GetTypeId())
	}
}

// TestLookupRegistrationMultiple tests lookup with multiple registrations.
func TestLookupRegistrationMultiple(t *testing.T) {
	r := NewObjectTypeRegistryResource()

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "plugin-a/type-one",
			RegistrationId: 1,
			PluginId:       "plugin-a",
		}
		r.registrations[2] = &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "plugin-b/type-two",
			RegistrationId: 2,
			PluginId:       "plugin-b",
		}
		r.registrations[3] = &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "plugin-a/type-three",
			RegistrationId: 3,
			PluginId:       "plugin-a",
		}
		broadcast()
	})

	reg := r.LookupRegistration("plugin-b/type-two")
	if reg == nil {
		t.Fatal("expected to find plugin-b/type-two")
	}
	if reg.GetRegistrationId() != 2 {
		t.Fatalf("expected registration_id 2, got %d", reg.GetRegistrationId())
	}

	reg = r.LookupRegistration("plugin-a/type-three")
	if reg == nil {
		t.Fatal("expected to find plugin-a/type-three")
	}
	if reg.GetRegistrationId() != 3 {
		t.Fatalf("expected registration_id 3, got %d", reg.GetRegistrationId())
	}

	reg = r.LookupRegistration("nonexistent/type")
	if reg != nil {
		t.Fatal("expected nil for nonexistent type")
	}
}

// TestGetRegistrationsLocked tests the snapshot helper.
func TestGetRegistrationsLocked(t *testing.T) {
	r := NewObjectTypeRegistryResource()

	// Empty registry should return empty slice.
	var regs []*s4wave_objecttype_registry.ObjectTypeRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		regs = r.getRegistrationsLocked()
	})
	if len(regs) != 0 {
		t.Fatalf("expected 0 registrations, got %d", len(regs))
	}

	// Add two registrations.
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "p/a",
			RegistrationId: 1,
			PluginId:       "p",
		}
		r.registrations[2] = &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "p/b",
			RegistrationId: 2,
			PluginId:       "p",
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
	r := NewObjectTypeRegistryResource()

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "test-plugin/removable",
			RegistrationId: 1,
			PluginId:       "test-plugin",
		}
		broadcast()
	})

	reg := r.LookupRegistration("test-plugin/removable")
	if reg == nil {
		t.Fatal("expected registration before removal")
	}

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(r.registrations, 1)
		broadcast()
	})

	reg = r.LookupRegistration("test-plugin/removable")
	if reg != nil {
		t.Fatal("expected nil after removal")
	}
}

// TestBroadcastOnChange tests that the broadcast channel fires when registrations change.
func TestBroadcastOnChange(t *testing.T) {
	r := NewObjectTypeRegistryResource()

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
		r.registrations[1] = &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "test-plugin/broadcast",
			RegistrationId: 1,
			PluginId:       "test-plugin",
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
