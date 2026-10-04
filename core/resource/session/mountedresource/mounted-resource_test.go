package mountedresource

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
)

func TestAddRegistersResourceValue(t *testing.T) {
	// Set up a resource context, a mounted value, and observable release state.
	resourceCtx := newResourceContext(context.Background())
	value := &struct {
		name string
	}{name: "mounted"}
	released := false

	// Register the mounted value and release callback with the resource context.
	id, err := Add(resourceCtx, srpc.NewMux(), value, func() {
		released = true
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read the value back through the resource context API.
	got, err := resourceCtx.GetResourceValue(id)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Assert that the registered resource retains the exact mounted value.
	if got != value {
		t.Fatalf("resource value = %T, want mounted value", got)
	}

	// Release the registered resource through its context.
	if !resourceCtx.ReleaseResource(id) {
		t.Fatal("expected release to succeed")
	}

	// Assert that releasing the resource invokes its cleanup callback.
	if !released {
		t.Fatal("expected release callback")
	}
}

type resourceContext struct {
	ctx      context.Context
	nextID   uint32
	values   map[uint32]any
	releases map[uint32]func()
}

func newResourceContext(ctx context.Context) *resourceContext {
	return &resourceContext{
		ctx:      ctx,
		values:   make(map[uint32]any),
		releases: make(map[uint32]func()),
	}
}

func (c *resourceContext) Context() context.Context {
	return c.ctx
}

func (c *resourceContext) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

func (c *resourceContext) AddResourceValue(_ srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	// Store each mounted value and its optional release callback under a fresh ID.
	c.nextID++
	id := c.nextID
	c.values[id] = value
	if releaseFn != nil {
		c.releases[id] = releaseFn
	}
	return id, nil
}

func (c *resourceContext) ReleaseResource(resourceID uint32) bool {
	// Remove the resource entry and run its release callback when present.
	_, ok := c.values[resourceID]
	if !ok {
		return false
	}
	delete(c.values, resourceID)
	if release := c.releases[resourceID]; release != nil {
		release()
		delete(c.releases, resourceID)
	}
	return true
}

func (c *resourceContext) GetResourceValue(resourceID uint32) (any, error) {
	value := c.values[resourceID]
	if value == nil {
		return nil, resource.ErrResourceNotFound
	}
	return value, nil
}

func (c *resourceContext) GetAttachedResource(uint32) (srpc.Client, error) {
	return nil, resource.ErrResourceNotFound
}
