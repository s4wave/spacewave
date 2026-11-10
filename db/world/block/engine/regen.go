package world_block_engine

import (
	"context"
	"math"
	"time"

	"github.com/pkg/errors"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// Regeneration defaults when the config leaves a field zero.
const (
	defaultRegenBudget      = 256 << 20
	defaultRegenSwapChanges = 4096
)

// Validate checks the regeneration config: a target that builds needs a
// directory for the sort's runs.
func (c *RegenConfig) Validate() error {
	switch c.GetTarget() {
	case RegenTarget_REGEN_TARGET_NONE, RegenTarget_REGEN_TARGET_SWITCHED_BACK, RegenTarget_REGEN_TARGET_RELEASED:
		return nil
	case RegenTarget_REGEN_TARGET_SWITCHED, RegenTarget_REGEN_TARGET_DONE:
		if c.GetSpillDir() == "" {
			return errors.New("spill_dir required to build")
		}
		return nil
	default:
		return errors.Errorf("unknown target %v", c.GetTarget())
	}
}

// buildConfig returns the Regenerator's config with the defaults applied.
func (c *RegenConfig) buildConfig() world_block.RegenConfig {
	cfg := world_block.RegenConfig{
		SpillDir:    c.GetSpillDir(),
		Budget:      int(min(c.GetBudgetBytes(), math.MaxInt)),
		SwapChanges: int(c.GetSwapChanges()),
	}
	if cfg.Budget == 0 {
		cfg.Budget = defaultRegenBudget
	}
	if cfg.SwapChanges == 0 {
		cfg.SwapChanges = defaultRegenSwapChanges
	}
	return cfg
}

// driveRegen moves the World a governs to the configured target, logging
// each stage it reaches, until the target is reached, ctx ends, or a step
// fails. Every step lands in the World's roots, so a stopped regeneration
// resumes from its last step the next time it runs.
func driveRegen(ctx context.Context, le *logrus.Entry, a *world_block.LocalAuthority, conf *RegenConfig) {
	// Report where the regeneration starts.
	r := world_block.NewRegenerator(a, conf.buildConfig())
	le = le.WithField("regen-target", conf.GetTarget().String())
	stage, err := r.Stage(ctx)
	if err == nil {
		le.WithField("regen-stage", stage.String()).Info("world regeneration starting")
		err = regenTo(ctx, le, r, conf.GetTarget(), stage)
	}
	if err != nil {
		if ctx.Err() == nil {
			le.WithError(err).Warn("world regeneration stopped")
		}
		return
	}

	// Report the stage it reached.
	if stage, err = r.Stage(ctx); err == nil {
		le.WithField("regen-stage", stage.String()).Info("world regeneration reached its target")
	}
}

// regenTo takes r from stage to target.
func regenTo(ctx context.Context, le *logrus.Entry, r *world_block.Regenerator, target RegenTarget, stage world_block.RegenStage) error {
	// Switch back or release in one call; neither has work on a World with no
	// regeneration.
	idle := stage == world_block.RegenIdle || stage == world_block.RegenDone
	switch target {
	case RegenTarget_REGEN_TARGET_NONE:
		return nil
	case RegenTarget_REGEN_TARGET_SWITCHED_BACK:
		if idle {
			return nil
		}
		return r.SwitchBack(ctx)
	case RegenTarget_REGEN_TARGET_RELEASED:
		if idle {
			return nil
		}
		return r.Release(ctx)
	}

	// Step forward to switched or done, logging each step.
	until := world_block.RegenDone
	if target == RegenTarget_REGEN_TARGET_SWITCHED {
		until = world_block.RegenSwitched
	}
	return r.Run(ctx, until, func(stage world_block.RegenStage, took time.Duration) {
		le.WithFields(logrus.Fields{
			"regen-stage": stage.String(),
			"took":        took.Round(time.Millisecond).String(),
		}).Info("world regeneration step")
	})
}
