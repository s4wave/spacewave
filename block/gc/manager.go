package block_gc

import (
	"context"
	"runtime/trace"
	"time"

	"github.com/pkg/errors"
)

// ManagerConfig holds the configuration for a GC manager.
type ManagerConfig struct {
	SweepConfig
	// SweepInterval is the periodic sweep interval. Default 30s.
	SweepInterval time.Duration
}

// Manager owns the GC graph store and sweep executor lifecycle.
// It runs startup WAL replay and periodic sweep cycles.
type Manager struct {
	cfg ManagerConfig
}

// NewManager creates a GC manager with the given configuration.
func NewManager(cfg ManagerConfig) *Manager {
	if cfg.SweepInterval == 0 {
		cfg.SweepInterval = 30 * time.Second
	}
	return &Manager{cfg: cfg}
}

// Run starts the GC manager lifecycle: startup WAL replay followed
// by periodic sweep cycles. Blocks until the context is canceled.
func (m *Manager) Run(ctx context.Context) error {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/manager")
	defer task.End()

	// Startup: replay any remaining WAL files from previous sessions.
	_, replayTask := trace.NewTask(ctx, "hydra/block-gc/manager/startup-replay")
	n, err := m.cfg.ReplayWAL(ctx, m.cfg.Graph)
	replayTask.End()
	if err != nil {
		return errors.Wrap(err, "startup WAL replay")
	}
	if n > 0 {
		trace.Logf(ctx, "gc-manager", "replayed %d WAL entries on startup", n)
	}

	// Periodic sweep loop.
	timer := time.NewTimer(m.cfg.SweepInterval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		_, err := SweepCycle(ctx, m.cfg.SweepConfig)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			trace.Logf(ctx, "gc-manager", "sweep failed: %v", err)
		}
		timer.Reset(m.cfg.SweepInterval)
	}
}
