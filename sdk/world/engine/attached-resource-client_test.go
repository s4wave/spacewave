package sdk_world_engine_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

func TestAttachedResourceClientUsesAttachedResourceLifetime(t *testing.T) {
	ctx := context.Background()
	srpcClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewMux())))
	resourceCtx := &attachedResourceTestContext{
		ctx:       ctx,
		resources: map[uint32]srpc.Client{42: srpcClient},
		releases:  make(map[uint32]int),
	}

	client := sdk_world_engine.NewAttachedResourceClient(resourceCtx)
	ref := client.CreateResourceReference(42)
	if ref.GetResourceID() != 42 {
		t.Fatalf("resource id: got %d want 42", ref.GetResourceID())
	}
	got, err := ref.GetClient()
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if got == nil {
		t.Fatal("GetClient returned nil client")
	}

	ref.Release()
	ref.Release()
	if got := resourceCtx.releases[42]; got != 1 {
		t.Fatalf("release count: got %d want 1", got)
	}
	if _, err := ref.GetClient(); !errors.Is(err, resource.ErrResourceOrClientReleased) {
		t.Fatalf("released GetClient error: got %v", err)
	}
}

func TestNewAttachedEngineRequiresResourceContext(t *testing.T) {
	if _, err := sdk_world_engine.NewAttachedEngine(context.Background(), 1); !errors.Is(err, resource.ErrNoResourceClientContext) {
		t.Fatalf("NewAttachedEngine without resource context: got %v", err)
	}
}

type attachedResourceTestContext struct {
	ctx       context.Context
	resources map[uint32]srpc.Client
	releases  map[uint32]int
}

func (c *attachedResourceTestContext) Context() context.Context {
	return c.ctx
}

func (c *attachedResourceTestContext) AddResource(srpc.Invoker, func()) (uint32, error) {
	return 0, resource.ErrResourceNotFound
}

func (c *attachedResourceTestContext) AddResourceValue(srpc.Invoker, any, func()) (uint32, error) {
	return 0, resource.ErrResourceNotFound
}

func (c *attachedResourceTestContext) ReleaseResource(resourceID uint32) bool {
	if _, ok := c.resources[resourceID]; !ok {
		return false
	}
	delete(c.resources, resourceID)
	c.releases[resourceID]++
	return true
}

func (c *attachedResourceTestContext) GetResourceValue(uint32) (any, error) {
	return nil, resource.ErrResourceNotFound
}

func (c *attachedResourceTestContext) GetAttachedResource(resourceID uint32) (srpc.Client, error) {
	client := c.resources[resourceID]
	if client == nil {
		return nil, resource.ErrResourceNotFound
	}
	return client, nil
}

var _ resource_server.ResourceClientContext = (*attachedResourceTestContext)(nil)
