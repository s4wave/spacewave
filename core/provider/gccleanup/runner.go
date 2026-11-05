// Package provider_gccleanup runs provider account GC cleanup sweeps.
package provider_gccleanup

import (
	"context"
	"errors"

	"github.com/aperturerobotics/util/broadcast"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/sirupsen/logrus"
)

// CollectFunc runs one collection pass.
type CollectFunc func(context.Context) (block_gc.CollectResult, error)

// Runner serializes and coalesces provider account cleanup triggers.
type Runner struct {
	le         *logrus.Entry
	logMessage string
	collect    CollectFunc

	bcast               broadcast.Broadcast
	generation          uint64
	completedGeneration uint64
}

// NewRunner constructs a cleanup runner.
func NewRunner(
	le *logrus.Entry,
	logMessage string,
	collect CollectFunc,
) *Runner {
	return &Runner{
		le:         le,
		logMessage: logMessage,
		collect:    collect,
	}
}

// Trigger records that cleanup is needed and returns the requested generation.
func (r *Runner) Trigger() uint64 {
	var generation uint64
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.generation++
		generation = r.generation
		broadcast()
	})
	return generation
}

// CompletedGeneration returns the last fully completed cleanup generation.
func (r *Runner) CompletedGeneration() uint64 {
	var generation uint64
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		generation = r.completedGeneration
	})
	return generation
}

// Wait waits for cleanup work that was pending when Wait was called.
func (r *Runner) Wait(ctx context.Context) error {
	var target uint64
	for {
		var (
			done   bool
			waitCh <-chan struct{}
		)
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if target == 0 {
				target = r.generation
			}
			done = r.completedGeneration >= target
			if !done {
				waitCh = getWaitCh()
			}
		})
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// Drain waits until no cleanup work is pending at an observation point.
func (r *Runner) Drain(ctx context.Context) error {
	for {
		var (
			done   bool
			waitCh <-chan struct{}
		)
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			done = r.completedGeneration >= r.generation
			if !done {
				waitCh = getWaitCh()
			}
		})
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// Run waits for cleanup triggers and runs serialized sweeps until ctx is canceled.
func (r *Runner) Run(ctx context.Context) error {
	for {
		generation, waitCh := r.nextGeneration()
		if generation == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-waitCh:
				continue
			}
		}

		stats, err := r.collect(ctx)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			return err
		}
		r.logStats(stats)
		r.completeGeneration(generation)
	}
}

func (r *Runner) nextGeneration() (uint64, <-chan struct{}) {
	var (
		generation uint64
		waitCh     <-chan struct{}
	)
	r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		if r.generation > r.completedGeneration {
			generation = r.generation
			return
		}
		waitCh = getWaitCh()
	})
	return generation, waitCh
}

func (r *Runner) completeGeneration(generation uint64) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if generation > r.completedGeneration {
			r.completedGeneration = generation
			broadcast()
		}
	})
}

func (r *Runner) logStats(res block_gc.CollectResult) {
	if res.Freed == 0 && res.Lapsed == 0 {
		return
	}
	r.le.WithField("freed", res.Freed).
		WithField("lapsed", res.Lapsed).
		WithField("netted", res.Netted).
		WithField("duration", res.Duration.String()).
		Info(r.logMessage)
}
