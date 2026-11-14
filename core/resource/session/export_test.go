package resource_session

import (
	"context"
	"errors"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

// WaitTransferDone waits for the transfer goroutine to finish completely
// (including the source account delete of a Merge or Migrate).
func (r *SessionResource) WaitTransferDone(ctx context.Context) error {
	// Wait for the transfer routine to exit, ignoring cancellation after it has stopped.
	r.transferMgr.mtx.Lock()
	rc := r.transferMgr.rc
	r.transferMgr.mtx.Unlock()
	if rc == nil {
		return nil
	}
	if err := rc.WaitExited(ctx, true, nil); err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// SetLegacyDelete selects the legacy account delete until the returned restore
// runs.
func SetLegacyDelete(legacy bool) (restore func()) {
	prev := legacyDelete
	legacyDelete = legacy
	return func() { legacyDelete = prev }
}

// DeleteProviderAccountWithin runs the account delete with deleteTimeout set
// to timeout.
func DeleteProviderAccountWithin(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sessCtrl session.SessionController,
	providerID, accountID, objectStoreID string,
	vol volume.Volume,
	timeout time.Duration,
) error {
	prev := deleteTimeout
	deleteTimeout = timeout
	defer func() { deleteTimeout = prev }()
	return deleteProviderAccount(ctx, le, b, sessCtrl, providerID, accountID, objectStoreID, vol, nil)
}

// DeleteStep is a point in an account delete a test stops it after.
type DeleteStep = deleteStep

// Account delete steps a test stops a delete after.
const (
	StepMarked = stepMarked
	StepKeys   = stepKeys
	StepVolume = stepVolume
)

// ErrStopped is what a delete stopped by StopAfter returns.
var ErrStopped = errors.New("account delete stopped by the test")

// StopAfter stops each account delete after step, as a crash there would,
// until the returned restore runs.
func StopAfter(step deleteStep) (restore func()) {
	prev := stopAfter
	stopAfter = func(at deleteStep) error {
		if at == step {
			return ErrStopped
		}
		return nil
	}
	return func() { stopAfter = prev }
}
