package resource_session

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	"github.com/sirupsen/logrus"
)

func TestAddMountedSpaceResourceRegistersResourceValue(t *testing.T) {
	resourceCtx := newCreateSpaceResourceContext(context.Background())
	spaceResource := resource_space.NewSpaceResource(logrus.NewEntry(logrus.New()), nil, nil)
	released := false

	id, err := addMountedSpaceResource(resourceCtx, spaceResource, func() {
		released = true
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	value, err := resourceCtx.GetResourceValue(id)
	if err != nil {
		t.Fatal(err.Error())
	}
	if value != spaceResource {
		t.Fatalf("resource value = %T, want mounted SpaceResource", value)
	}
	if !resourceCtx.ReleaseResource(id) {
		t.Fatal("expected release to succeed")
	}
	if !released {
		t.Fatal("expected release callback")
	}
}

type createSpaceResourceContext struct {
	ctx      context.Context
	nextID   uint32
	values   map[uint32]any
	releases map[uint32]func()
}

func newCreateSpaceResourceContext(ctx context.Context) *createSpaceResourceContext {
	return &createSpaceResourceContext{
		ctx:      ctx,
		values:   make(map[uint32]any),
		releases: make(map[uint32]func()),
	}
}

func (c *createSpaceResourceContext) Context() context.Context {
	return c.ctx
}

func (c *createSpaceResourceContext) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

func (c *createSpaceResourceContext) AddResourceValue(_ srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	c.nextID++
	id := c.nextID
	c.values[id] = value
	if releaseFn != nil {
		c.releases[id] = releaseFn
	}
	return id, nil
}

func (c *createSpaceResourceContext) ReleaseResource(resourceID uint32) bool {
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

func (c *createSpaceResourceContext) GetResourceValue(resourceID uint32) (any, error) {
	value := c.values[resourceID]
	if value == nil {
		return nil, resource.ErrResourceNotFound
	}
	return value, nil
}

func (c *createSpaceResourceContext) GetAttachedResource(uint32) (srpc.Client, error) {
	return nil, resource.ErrResourceNotFound
}
