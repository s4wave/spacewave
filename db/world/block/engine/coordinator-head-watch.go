package world_block_engine

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/object"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

func (c *Controller) startCoordinatorHeadWatch(
	ctx context.Context,
	coordinator coord.Coordinator,
	scope coord.Scope,
	stateStore object.ObjectStore,
	engine *world_block.Engine,
) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.runCoordinatorHeadWatch(ctx, coordinator, scope, stateStore, engine)
	}()
	return done
}

func (c *Controller) runCoordinatorHeadWatch(
	ctx context.Context,
	coordinator coord.Coordinator,
	scope coord.Scope,
	stateStore object.ObjectStore,
	engine *world_block.Engine,
) {
	capability, err := coordinator.Capability(ctx, scope)
	if err != nil {
		if ctx.Err() == nil {
			c.le.WithError(err).Warn("world coordinator capability lookup failed")
		}
		return
	}
	if !capability.Supported {
		return
	}
	c.refreshHeadFromCoordinatorEvent(ctx, stateStore, engine, coord.Event{
		VolumeID:      scope.VolumeID,
		ObjectStoreID: scope.ObjectStoreID,
		Generation:    capability.Generation,
	})

	watch, err := coordinator.Watch(ctx, scope, capability.Generation)
	if err != nil {
		if ctx.Err() == nil && !errors.Is(err, coord.ErrUnsupported) {
			c.le.WithError(err).Warn("world coordinator watch failed")
		}
		return
	}
	defer watch.Close()
	c.refreshHeadFromCoordinatorEvent(ctx, stateStore, engine, coord.Event{
		VolumeID:      scope.VolumeID,
		ObjectStoreID: scope.ObjectStoreID,
		Generation:    capability.Generation,
	})

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watch.Events():
			if !ok {
				return
			}
			c.refreshHeadFromCoordinatorEvent(ctx, stateStore, engine, event)
		}
	}
}

func (c *Controller) refreshHeadFromCoordinatorEvent(
	ctx context.Context,
	stateStore object.ObjectStore,
	engine *world_block.Engine,
	event coord.Event,
) {
	headRef, err := c.refreshDurableHeadRef(stateStore)(ctx)
	if err != nil {
		if ctx.Err() == nil {
			c.le.WithError(err).WithField("generation", event.Generation).Warn("world head refresh failed")
		}
		return
	}
	if headRef == nil || headRef.GetRootRef().GetEmpty() {
		return
	}
	if err := engine.AdoptRootRefFromWatch(ctx, headRef); err != nil && ctx.Err() == nil {
		c.le.WithError(err).WithField("generation", event.Generation).Warn("world head adoption failed")
	}
}

func (c *Controller) refreshDurableHeadRef(stateStore object.ObjectStore) func(context.Context) (*bucket.ObjectRef, error) {
	return func(ctx context.Context) (*bucket.ObjectRef, error) {
		headState, found, err := c.loadHeadState(ctx, stateStore)
		if err != nil || !found {
			return nil, err
		}
		return headState.GetHeadRef(), nil
	}
}
