//go:build !js

package coord

import (
	"context"

	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/sirupsen/logrus"
)

// EngineFactory creates a world engine when this process becomes leader.
// The ctx is cancelled when leadership is lost.
type EngineFactory func(ctx context.Context) (world.Engine, error)

// WorldRoleHandler implements RoleChangeHandler to manage the world
// engine lifecycle based on the coordinator's leader/follower role.
type WorldRoleHandler struct {
	le            *logrus.Entry
	engineFactory EngineFactory

	bcast  broadcast.Broadcast
	engine world.Engine
}

// NewWorldRoleHandler creates a new WorldRoleHandler.
func NewWorldRoleHandler(le *logrus.Entry, factory EngineFactory) *WorldRoleHandler {
	return &WorldRoleHandler{
		le:            le,
		engineFactory: factory,
	}
}

// OnBecomeLeader creates the world engine and blocks until leadership is lost.
func (h *WorldRoleHandler) OnBecomeLeader(ctx context.Context) error {
	eng, err := h.engineFactory(ctx)
	if err != nil {
		return err
	}
	h.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		h.engine = eng
		broadcast()
	})
	h.le.Info("world engine started as leader")
	<-ctx.Done()
	h.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		h.engine = nil
		broadcast()
	})
	h.le.Info("world engine stopped")
	return nil
}

// OnBecomeFollower blocks until the follower context is cancelled.
func (h *WorldRoleHandler) OnBecomeFollower(ctx context.Context, leaderSocketPath string) error {
	<-ctx.Done()
	return nil
}

// GetEngine returns the current world engine, or nil if not leader.
func (h *WorldRoleHandler) GetEngine() world.Engine {
	var eng world.Engine
	h.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		eng = h.engine
	})
	return eng
}

// WaitEngine waits for the world engine to become available.
func (h *WorldRoleHandler) WaitEngine(ctx context.Context) (world.Engine, error) {
	var eng world.Engine
	err := h.bcast.Wait(ctx, func(broadcast func(), getWaitCh func() <-chan struct{}) (bool, error) {
		eng = h.engine
		return eng != nil, nil
	})
	return eng, err
}

// _ is a type assertion.
var _ RoleChangeHandler = (*WorldRoleHandler)(nil)
