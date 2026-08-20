package resource_objecttype_registry

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
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

func TestRegisterObjectTypeRejectsDuplicateTypeID(t *testing.T) {
	r := NewObjectTypeRegistryResource()
	r.registrations[1] = &objectTypeRegistration{registration: &s4wave_objecttype_registry.ObjectTypeRegistration{
		TypeId:         "test-plugin/duplicate",
		RegistrationId: 1,
		PluginId:       "test-plugin",
	}}
	r.nextID = 2
	client := &testResourceClientContext{ctx: t.Context()}

	_, err := r.RegisterObjectType(
		resource_server.WithResourceClientContext(t.Context(), client),
		&s4wave_objecttype_registry.RegisterObjectTypeRequest{
			TypeId:   "test-plugin/duplicate",
			PluginId: "other-plugin",
		},
	)
	if err != ErrTypeIdAlreadyRegistered {
		t.Fatalf("RegisterObjectType() error = %v, want %v", err, ErrTypeIdAlreadyRegistered)
	}
	if r.nextID != 2 {
		t.Fatalf("next registration ID = %d, want 2", r.nextID)
	}
	registration := r.LookupRegistration("test-plugin/duplicate")
	if registration.GetPluginId() != "test-plugin" {
		t.Fatalf("duplicate changed plugin ID to %q", registration.GetPluginId())
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
		r.registrations[1] = &objectTypeRegistration{registration: &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "test-plugin/test-type",
			RegistrationId: 1,
			PluginId:       "test-plugin",
			Metadata: &s4wave_objecttype_registry.ObjectTypeMetadata{
				DisplayName: "Test Type",
				IconName:    "box",
				Visibility:  s4wave_objecttype_registry.ObjectTypeVisibility_OBJECT_TYPE_VISIBILITY_VISIBLE,
				Description: "Test object type",
			},
		}}
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
	if reg.GetMetadata().GetDisplayName() != "Test Type" {
		t.Fatalf("expected display_name Test Type, got %s", reg.GetMetadata().GetDisplayName())
	}
	if reg.GetMetadata().GetIconName() != "box" {
		t.Fatalf("expected icon_name box, got %s", reg.GetMetadata().GetIconName())
	}
	if reg.GetMetadata().GetVisibility() != s4wave_objecttype_registry.ObjectTypeVisibility_OBJECT_TYPE_VISIBILITY_VISIBLE {
		t.Fatalf("expected visible metadata, got %s", reg.GetMetadata().GetVisibility().String())
	}
}

// TestLookupRegistrationReturnsClone tests that LookupRegistration returns a clone.
func TestLookupRegistrationReturnsClone(t *testing.T) {
	r := NewObjectTypeRegistryResource()

	orig := &s4wave_objecttype_registry.ObjectTypeRegistration{
		TypeId:         "test-plugin/cloned",
		RegistrationId: 1,
		PluginId:       "test-plugin",
		Metadata: &s4wave_objecttype_registry.ObjectTypeMetadata{
			DisplayName: "Clone Me",
			IconName:    "box",
			Visibility:  s4wave_objecttype_registry.ObjectTypeVisibility_OBJECT_TYPE_VISIBILITY_VISIBLE,
		},
	}
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &objectTypeRegistration{registration: orig}
		broadcast()
	})

	reg := r.LookupRegistration("test-plugin/cloned")
	if reg == nil {
		t.Fatal("expected non-nil registration")
	}

	// Mutating the returned value should not affect the stored one.
	reg.TypeId = "mutated"
	reg.Metadata.DisplayName = "mutated"
	reg2 := r.LookupRegistration("test-plugin/cloned")
	if reg2 == nil {
		t.Fatal("expected registration to still exist after mutating clone")
	}
	if reg2.GetTypeId() != "test-plugin/cloned" {
		t.Fatalf("stored registration was mutated: got %s", reg2.GetTypeId())
	}
	if reg2.GetMetadata().GetDisplayName() != "Clone Me" {
		t.Fatalf("stored metadata was mutated: got %s", reg2.GetMetadata().GetDisplayName())
	}
}

// TestLookupRegistrationMultiple tests lookup with multiple registrations.
func TestLookupRegistrationMultiple(t *testing.T) {
	r := NewObjectTypeRegistryResource()

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.registrations[1] = &objectTypeRegistration{registration: &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "plugin-a/type-one",
			RegistrationId: 1,
			PluginId:       "plugin-a",
		}}
		r.registrations[2] = &objectTypeRegistration{registration: &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "plugin-b/type-two",
			RegistrationId: 2,
			PluginId:       "plugin-b",
		}}
		r.registrations[3] = &objectTypeRegistration{registration: &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "plugin-a/type-three",
			RegistrationId: 3,
			PluginId:       "plugin-a",
		}}
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
		r.registrations[1] = &objectTypeRegistration{registration: &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "p/a",
			RegistrationId: 1,
			PluginId:       "p",
		}}
		r.registrations[2] = &objectTypeRegistration{registration: &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "p/b",
			RegistrationId: 2,
			PluginId:       "p",
		}}
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
		r.registrations[1] = &objectTypeRegistration{registration: &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "test-plugin/removable",
			RegistrationId: 1,
			PluginId:       "test-plugin",
		}}
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
		r.registrations[1] = &objectTypeRegistration{registration: &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         "test-plugin/broadcast",
			RegistrationId: 1,
			PluginId:       "test-plugin",
		}}
		broadcast()
	})

	// Channel should now be closed.
	select {
	case <-waitCh:
	default:
		t.Fatal("wait channel not closed after broadcast")
	}
}

func TestRegisterObjectTypeAttachedHandlerRequiresOwnedResource(t *testing.T) {
	ctx := t.Context()
	registry, resources, registryClient := newRegistryResourceClient(t, ctx)
	defer resources.Release()

	if _, err := registryClient.RegisterObjectType(ctx, &s4wave_objecttype_registry.RegisterObjectTypeRequest{
		TypeId:                    "test/unowned",
		PluginId:                  "test-plugin",
		AttachedHandlerResourceId: 99,
	}); err == nil {
		t.Fatal("RegisterObjectType accepted an unowned attached handler")
	}
	if registry.LookupRegistration("test/unowned") != nil {
		t.Fatal("unowned attached handler created a registration")
	}

	resp, err := registryClient.RegisterObjectType(ctx, &s4wave_objecttype_registry.RegisterObjectTypeRequest{
		TypeId:   "test/plugin",
		PluginId: "test-plugin",
	})
	if err != nil {
		t.Fatalf("RegisterObjectType ordinary registration: %v", err)
	}
	resources.CreateResourceReference(resp.GetResourceId()).Release()
}

func TestRegisterObjectTypeAttachedHandlerEndsWithClientGeneration(t *testing.T) {
	ctx := t.Context()
	registry, resources, registryClient := newRegistryResourceClient(t, ctx)

	serviceMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(srpc.NewMux()).Register(serviceMux); err != nil {
		t.Fatalf("register attached ResourceService: %v", err)
	}
	attachedID, err := resources.AttachResource(ctx, "handler", serviceMux)
	if err != nil {
		t.Fatalf("AttachResource: %v", err)
	}
	_, err = registryClient.RegisterObjectType(ctx, &s4wave_objecttype_registry.RegisterObjectTypeRequest{
		TypeId:                    "test/attached",
		PluginId:                  "test-plugin",
		AttachedHandlerResourceId: attachedID,
	})
	if err != nil {
		t.Fatalf("RegisterObjectType attached handler: %v", err)
	}
	if registry.LookupRegistration("test/attached") == nil {
		t.Fatal("attached handler registration is not visible")
	}

	waitCh := registryChangeWait(registry)
	resources.Release()
	select {
	case <-waitCh:
	case <-ctx.Done():
		t.Fatal("client generation close did not reach registry")
	}
	if registry.LookupRegistration("test/attached") != nil {
		t.Fatal("registration remained after its client generation closed")
	}
}

func newRegistryResourceClient(
	t *testing.T,
	ctx context.Context,
) (*ObjectTypeRegistryResource, *resource_client.Client, s4wave_objecttype_registry.SRPCObjectTypeRegistryResourceServiceClient) {
	t.Helper()
	registry := NewObjectTypeRegistryResource()
	serviceMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(registry.GetMux()).Register(serviceMux); err != nil {
		t.Fatalf("register registry ResourceService: %v", err)
	}
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serviceMux)))
	resources, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(client))
	if err != nil {
		t.Fatalf("new registry ResourceClient: %v", err)
	}
	rootRef := resources.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		resources.Release()
		t.Fatalf("access registry root: %v", err)
	}
	t.Cleanup(rootRef.Release)
	return registry, resources, s4wave_objecttype_registry.NewSRPCObjectTypeRegistryResourceServiceClient(rootClient)
}

func registryChangeWait(r *ObjectTypeRegistryResource) <-chan struct{} {
	var waitCh <-chan struct{}
	r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
	})
	return waitCh
}
