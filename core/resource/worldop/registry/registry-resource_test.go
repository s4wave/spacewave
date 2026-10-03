package resource_worldop_registry

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
)

// TestNewWorldOpRegistryResource tests basic construction.
func TestNewWorldOpRegistryResource(t *testing.T) {
	// Construct the operation registry and verify its initial resources.
	r := NewWorldOpRegistryResource(nil)
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
	r := NewWorldOpRegistryResource(nil)
	reg := r.LookupRegistrationByOpType("unknown/op", "")
	if reg != nil {
		t.Fatal("expected nil for unknown operation type")
	}
}

// TestLookupRegistrationByOpTypeFound tests that LookupRegistrationByOpType finds a manually added registration.
func TestLookupRegistrationByOpTypeFound(t *testing.T) {
	// Construct an empty operation registry for lookup.
	r := NewWorldOpRegistryResource(nil)

	// Publish the test operation registration under the registry lock.
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_worldop_registry.WorldOpRegistration{
			OperationTypeId: "test-plugin/test-op",
			RegistrationId:  1,
			PluginId:        "test-plugin",
		}
		broadcast()
	})

	// Look up the test operation and verify its registration fields.
	reg := r.LookupRegistrationByOpType("test-plugin/test-op", "")
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
	// Construct the registry whose returned registration will be mutated.
	r := NewWorldOpRegistryResource(nil)

	// Publish the original operation registration for the clone check.
	orig := &s4wave_worldop_registry.WorldOpRegistration{
		OperationTypeId: "test-plugin/cloned",
		RegistrationId:  1,
		PluginId:        "test-plugin",
	}
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = orig
		broadcast()
	})

	// Require the original operation registration to be available for lookup.
	reg := r.LookupRegistrationByOpType("test-plugin/cloned", "")
	if reg == nil {
		t.Fatal("expected non-nil registration")
	}

	// Mutating the returned value should not affect the stored one.
	reg.OperationTypeId = "mutated"
	reg2 := r.LookupRegistrationByOpType("test-plugin/cloned", "")
	if reg2 == nil {
		t.Fatal("expected registration to still exist after mutating clone")
	}
	if reg2.GetOperationTypeId() != "test-plugin/cloned" {
		t.Fatalf("stored registration was mutated: got %s", reg2.GetOperationTypeId())
	}
}

// TestLookupRegistrationByOpTypeMultiple tests lookup with multiple registrations.
func TestLookupRegistrationByOpTypeMultiple(t *testing.T) {
	// Construct the registry for lookups across multiple plugins.
	r := NewWorldOpRegistryResource(nil)

	// Publish distinct operation registrations for both plugins.
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

	// Look up the second plugin operation and verify its registration identifier.
	reg := r.LookupRegistrationByOpType("plugin-b/op-two", "")
	if reg == nil {
		t.Fatal("expected to find plugin-b/op-two")
	}
	if reg.GetRegistrationId() != 2 {
		t.Fatalf("expected registration_id 2, got %d", reg.GetRegistrationId())
	}

	// Look up the other first-plugin operation and verify its identifier.
	reg = r.LookupRegistrationByOpType("plugin-a/op-three", "")
	if reg == nil {
		t.Fatal("expected to find plugin-a/op-three")
	}
	if reg.GetRegistrationId() != 3 {
		t.Fatalf("expected registration_id 3, got %d", reg.GetRegistrationId())
	}

	// Verify an unregistered operation remains absent.
	reg = r.LookupRegistrationByOpType("nonexistent/op", "")
	if reg != nil {
		t.Fatal("expected nil for nonexistent operation type")
	}
}

// TestGetRegistrationsLocked tests the snapshot helper.
func TestGetRegistrationsLocked(t *testing.T) {
	// Construct the registry for registration snapshot checks.
	r := NewWorldOpRegistryResource(nil)

	// Empty registry should return empty slice.
	var regs []*s4wave_worldop_registry.WorldOpRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		regs = r.getRegistrationsLocked("")
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

	// Read the populated registry snapshot and verify both registrations appear.
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		regs = r.getRegistrationsLocked("")
	})
	if len(regs) != 2 {
		t.Fatalf("expected 2 registrations, got %d", len(regs))
	}
}

// TestRegistrationRemoval tests that deleting a registration makes it unfindable.
func TestRegistrationRemoval(t *testing.T) {
	// Construct the registry for operation removal checks.
	r := NewWorldOpRegistryResource(nil)

	// Publish the operation registration that will be removed.
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &s4wave_worldop_registry.WorldOpRegistration{
			OperationTypeId: "test-plugin/removable",
			RegistrationId:  1,
			PluginId:        "test-plugin",
		}
		broadcast()
	})

	// Verify the operation can be found before removing its registration.
	reg := r.LookupRegistrationByOpType("test-plugin/removable", "")
	if reg == nil {
		t.Fatal("expected registration before removal")
	}

	// Remove the operation registration and notify registry watchers.
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(r.registrations, 1)
		broadcast()
	})

	// Verify the removed operation can no longer be found.
	reg = r.LookupRegistrationByOpType("test-plugin/removable", "")
	if reg != nil {
		t.Fatal("expected nil after removal")
	}
}

// TestBroadcastOnChange tests that the broadcast channel fires when registrations change.
func TestBroadcastOnChange(t *testing.T) {
	// Construct the registry for operation change notifications.
	r := NewWorldOpRegistryResource(nil)

	// Subscribe to the next operation registry change under the registry lock.
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

// fakeResourceClientContext satisfies the Resource RPC client context for
// direct RegisterWorldOp calls in tests.
type fakeResourceClientContext struct{}

func (f *fakeResourceClientContext) Context() context.Context { return context.Background() }

func (f *fakeResourceClientContext) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return 1, nil
}

func (f *fakeResourceClientContext) AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	return 1, nil
}

func (f *fakeResourceClientContext) ReleaseResource(resourceID uint32) bool { return true }

func (f *fakeResourceClientContext) GetResourceValue(resourceID uint32) (any, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeResourceClientContext) GetAttachedResource(id uint32) (srpc.Client, error) {
	return nil, errors.New("not implemented")
}

// TestRegisterWorldOpRejectsDuplicateOperationType verifies a second
// registration of one operation type ID fails instead of making dispatch
// nondeterministic between plugins.
func TestRegisterWorldOpRejectsDuplicateOperationType(t *testing.T) {
	// Construct a client context and request for a duplicate operation registration.
	r := NewWorldOpRegistryResource(nil)
	ctx := resource_server.WithResourceClientContext(
		context.Background(),
		&fakeResourceClientContext{},
	)
	req := &s4wave_worldop_registry.RegisterWorldOpRequest{
		OperationTypeId: "test-plugin/duplicate",
		PluginId:        "test-plugin",
	}

	// Register the operation once and require the duplicate request to fail.
	if _, err := r.RegisterWorldOp(ctx, req); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if _, err := r.RegisterWorldOp(ctx, req); !errors.Is(err, ErrOperationTypeAlreadyRegistered) {
		t.Fatalf("duplicate registration error = %v, want ErrOperationTypeAlreadyRegistered", err)
	}

	// Verify duplicate rejection preserves the original operation registration.
	reg := r.LookupRegistrationByOpType("test-plugin/duplicate", "")
	if reg == nil || reg.GetRegistrationId() != 1 {
		t.Fatalf("original registration changed after duplicate attempt: %+v", reg)
	}
}
