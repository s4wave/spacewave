package execution_controller

import (
	"context"
	"time"

	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	"github.com/sirupsen/logrus"
)

// Lease renews one granted claim and cancels its operation ClaimClockSkew before
// the last committed expiry. Renewal and settlement stop permanently on a stale
// epoch. Reclaim cannot overlap operation when the reclaimer's clock lead plus
// operation's cancellation drain time stays within ClaimClockSkew.
type Lease struct {
	// le reports transient renewal failures.
	le *logrus.Entry
	// expiry is the initial committed lease deadline.
	expiry time.Time
	// duration is the extension requested every duration/3.
	duration time.Duration
	// renew commits a new expiry under the granted claim and honors cancellation.
	renew func(context.Context, time.Time) error
}

// NewLease constructs a lease for one granted claim with duration greater than
// ClaimClockSkew. renew must honor cancellation and report success only after the
// new expiry commits. Construction and Execute require an already granted claim.
func NewLease(le *logrus.Entry, expiry time.Time, duration time.Duration, renew func(context.Context, time.Time) error) *Lease {
	return &Lease{le: le, expiry: expiry, duration: duration, renew: renew}
}

// Execute runs and drains operation under the committed lease. A failed or
// blocked renewal cannot delay cancellation past expiry minus ClaimClockSkew.
// Losing the lease returns nil so a routine does not restart the same claim.
// Execute joins renewal before returning; renew must honor its context.
func (l *Lease) Execute(rctx context.Context, operation func(context.Context) error) error {
	// Fence execution before the published expiry, including on reconstruction.
	deadline := l.expiry.Add(-forge_execution.ClaimClockSkew)
	if !time.Now().Before(deadline) {
		return nil
	}
	ctx, cancel := context.WithCancel(rctx)
	defer cancel()
	fence := time.AfterFunc(time.Until(deadline), func() { cancel() })
	defer fence.Stop()

	// Keep renewal scoped to the operation and join it during shutdown.
	renewCtx, stopRenew := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		l.renewUntilCanceled(renewCtx, fence, cancel)
	}()
	defer func() {
		stopRenew()
		<-done
	}()

	// Report ordinary operation failures while making lease loss terminal.
	err := operation(ctx)
	if rctx.Err() != nil {
		return context.Canceled
	}
	if ctx.Err() != nil || execution_tx.IsClaimFenced(err) {
		return nil
	}
	return err
}

// renewUntilCanceled moves the cancellation deadline only after a committed
// extension. The deadline callback remains independent of a blocked renew call.
func (l *Lease) renewUntilCanceled(ctx context.Context, fence *time.Timer, cancel context.CancelFunc) {
	// Schedule each required lease extension without inspecting cached liveness.
	timer := time.NewTimer(time.Until(l.expiry.Add(-l.duration * 2 / 3)))
	defer timer.Stop()
	for {
		// Wait for extension demand or the last committed lease's cancellation.
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		// Publish the new expiry before extending local execution authority.
		expiry := time.Now().Add(l.duration)
		err := l.renew(ctx, expiry)
		switch {
		case ctx.Err() != nil:
			return
		case err == nil:
			fence.Reset(time.Until(expiry.Add(-forge_execution.ClaimClockSkew)))
		case execution_tx.IsClaimFenced(err):
			cancel()
			return
		default:
			l.le.WithError(err).Warn("renew execution claim lease")
		}

		// Request the next extension while the committed deadline remains armed.
		timer.Reset(l.duration / 3)
	}
}
