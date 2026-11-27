//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	os_system "github.com/s4wave/spacewave/os/system"
	os_system_stager "github.com/s4wave/spacewave/os/system/stager"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	"github.com/sirupsen/logrus"
)

// deviceUpdate computes the Device's update state and status from one
// source's latest report, given the Device as stored.
type deviceUpdate func(existing *s4wave_device.Device, now time.Time) (s4wave_device.DeviceUpdateState, *s4wave_device.DeviceStatus)

// deviceUpdateRetry bounds the wait before a failed projection retries: the
// Device object appears only once setup completes, and a write may fail.
const deviceUpdateRetry = 30 * time.Second

// startDeviceUpdateProjection starts the one producer of the Device's update
// state. It watches the launcher, which updates a desktop daemon, and the
// os/system node, which updates a Spacewave OS Device, and writes the latest
// report of either onto the Device object, one write at a time.
func startDeviceUpdateProjection(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	b bus.Bus,
	invoker srpc.Invoker,
) {
	if b == nil || invoker == nil {
		return
	}
	go func() {
		// Open the SDK client the projection writes the Device through.
		client, err := buildSDKClientFromInvoker(ctx, invoker)
		if err != nil {
			if ctx.Err() == nil {
				le.WithError(err).Warn("device update projection unavailable")
			}
			return
		}
		defer client.close()
		updates := make(chan deviceUpdate)
		go func() {
			if err := watchLauncherUpdates(ctx, b, updates); err != nil && ctx.Err() == nil {
				le.WithError(err).Warn("launcher update watch stopped")
			}
		}()
		go watchRevisionUpdates(ctx, le, b, updates)
		runDeviceUpdateProjection(ctx, le, statePath, client, updates)
	}()
}

// runDeviceUpdateProjection writes each update onto the Device until ctx
// ends. A later update replaces one not yet written, and a failed write
// retries after a backoff until it succeeds or a later update arrives.
func runDeviceUpdateProjection(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	client *sdkClient,
	updates <-chan deviceUpdate,
) {
	var pending deviceUpdate
	var retry <-chan time.Time
	delay := time.Second
	for {
		// Wait for an update or for the pending update to retry.
		select {
		case <-ctx.Done():
			return
		case pending = <-updates:
			delay = time.Second
		case <-retry:
		}

		// Write the pending update, or retry after a backoff.
		err := projectDeviceUpdate(ctx, statePath, client, pending, time.Now())
		if err == nil {
			pending, retry = nil, nil
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if stderrors.Is(err, world.ErrObjectNotFound) || stderrors.Is(err, errDeviceNotReady) {
			le.WithError(err).Debug("device object not available for update projection")
		} else {
			le.WithError(err).Warn("failed to project update state onto device")
		}
		//gostyle:timer backoff
		retry = time.After(delay)
		delay = min(delay*2, deviceUpdateRetry)
	}
}

// watchLauncherUpdates sends an update for each launcher info snapshot. It
// returns nil when the daemon has no launcher.
func watchLauncherUpdates(ctx context.Context, b bus.Bus, updates chan<- deviceUpdate) error {
	// Look up the launcher RPC service.
	invokers, _, invokerRef, err := bifrost_rpc.ExLookupRpcService(
		ctx,
		b,
		spacewave_launcher.PluginLauncherServiceID,
		"",
		true,
		nil,
	)
	if err != nil {
		return err
	}
	if len(invokers) == 0 {
		return nil
	}
	defer invokerRef.Release()

	// Watch launcher info from the first invoker.
	launcherClient := spacewave_launcher.NewSRPCLauncherClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invokers[0]))))
	strm, err := launcherClient.WatchLauncherInfo(ctx, &spacewave_launcher.WatchLauncherInfoRequest{})
	if err != nil {
		return err
	}
	defer strm.Close()
	for {
		info, err := strm.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case updates <- launcherUpdate(info):
		case <-ctx.Done():
			return nil
		}
	}
}

// watchRevisionUpdates sends an update for each report of the Device's
// revisions that its os/system node serves. It waits for a node to be
// placed, and watches again after the node stops, until ctx ends.
func watchRevisionUpdates(ctx context.Context, le *logrus.Entry, b bus.Bus, updates chan<- deviceUpdate) {
	stager := os_system_stager.NewSRPCStagerClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(os_system.NewInvoker(b)))))
	delay := time.Second
	for {
		// Send each report until the node's stream ends.
		err := func() error {
			strm, err := stager.WatchRevision(ctx, &os_system_stager.WatchRevisionRequest{})
			if err != nil {
				return err
			}
			defer strm.Close()
			for {
				rev, err := strm.Recv()
				if err != nil {
					return err
				}
				delay = time.Second
				select {
				case updates <- revisionUpdate(rev):
				case <-ctx.Done():
					return nil
				}
			}
		}()
		if ctx.Err() != nil {
			return
		}

		// The node stopped, so watch for the next node after a backoff.
		le.WithError(err).Debug("os/system revision watch ended")
		select {
		case <-ctx.Done():
			return
		//gostyle:timer backoff
		case <-time.After(delay):
		}
		delay = min(delay*2, deviceUpdateRetry)
	}
}

// errDeviceNotReady means the daemon's Device setup has not completed, so no
// Device object exists to project onto.
var errDeviceNotReady = stderrors.New("device setup not ready")

// projectDeviceUpdate writes the update onto the Device object setup selects.
func projectDeviceUpdate(
	ctx context.Context,
	statePath string,
	client *sdkClient,
	update deviceUpdate,
	now time.Time,
) error {
	// Read the ready setup record and decode its resource ID.
	record, ok, err := deviceUpdateProjectionTarget(statePath)
	if err != nil {
		return err
	}
	if !ok {
		return errDeviceNotReady
	}
	spaceID, err := decodeDeviceResourceID(record.ResourceID)
	if err != nil {
		return err
	}

	// Mount the Device session.
	sess, err := client.mountSession(ctx, record.SessionIndex)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Mount the Space service.
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		return err
	}
	defer spaceCleanup()

	// Open the World engine.
	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		return err
	}
	defer engineCleanup()

	// Open a write transaction.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	// Load the Device object and require its peer to match setup state.
	objState, found, err := tx.GetObject(ctx, record.DeviceObjectKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}
	existing, err := readDeviceBlock(ctx, objState)
	if err != nil {
		return err
	}
	if existing.GetPeerId() != record.PeerID {
		return errors.New("device object peer_id does not match setup state")
	}

	// Project the update onto the Device.
	next, changed, err := projectUpdateOntoDevice(existing, update, now)
	if err != nil || !changed {
		return err
	}

	// Write the updated Device block and commit.
	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(next, true)
		return nil
	})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func deviceUpdateProjectionTarget(statePath string) (*deviceSetupRecord, bool, error) {
	// Read a ready setup record with the fields the projection needs.
	record, err := readDeviceSetupRecord(statePath)
	if err != nil {
		return nil, false, err
	}
	if record.SetupState != deviceSetupStateSessionReady {
		return nil, false, nil
	}
	if record.PeerID == "" || record.ResourceID == "" || record.SessionIndex == 0 || record.DeviceObjectKey == "" {
		return nil, false, nil
	}
	return record, true, nil
}

// projectUpdateOntoDevice returns the Device with the update applied, and
// whether it changed.
func projectUpdateOntoDevice(
	existing *s4wave_device.Device,
	update deviceUpdate,
	now time.Time,
) (*s4wave_device.Device, bool, error) {
	// Clone the Device and compute the update projection.
	if existing == nil {
		return nil, false, errors.New("device state is required")
	}
	next := existing.CloneVT()
	updateState, status := update(existing, now)

	// Apply a changed update state and status, then validate the Device.
	changed := false
	if next.GetUpdateState() != updateState {
		next.UpdateState = updateState
		changed = true
	}
	if status != nil && !sameDeviceStatus(status, next.GetLastStatus()) {
		next.LastStatus = status
		changed = true
	}
	if !changed {
		return next, false, nil
	}
	next.UpdatedAt = timestamppb.New(now)
	if err := next.Validate(); err != nil {
		return nil, false, err
	}
	return next, true, nil
}

// launcherUpdate projects a launcher info snapshot.
func launcherUpdate(info *spacewave_launcher.LauncherInfo) deviceUpdate {
	return func(existing *s4wave_device.Device, now time.Time) (s4wave_device.DeviceUpdateState, *s4wave_device.DeviceStatus) {
		return deviceLauncherUpdateProjection(existing, info, now)
	}
}

// revisionUpdate projects a report of a Spacewave OS Device's revisions:
// staging, a trial booting, a trial that rolled back, or the accepted
// revision running. A Device that was updating and now only runs its
// accepted revision accepted that update.
func revisionUpdate(rev *os_system_stager.Revision) deviceUpdate {
	return func(existing *s4wave_device.Device, now time.Time) (s4wave_device.DeviceUpdateState, *s4wave_device.DeviceStatus) {
		// A Device that reports its revisions is online.
		status := &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			ObservedAt: timestamppb.New(now),
		}
		accepted := rev.GetAccepted().MarshalString()
		switch u := rev.GetUpdate().(type) {
		case *os_system_stager.Revision_Staging:
			status.Message = "staging update: " + u.Staging.MarshalString()
			return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_STAGING, status
		case *os_system_stager.Revision_Trial:
			status.Message = "booting update on trial: " + u.Trial.MarshalString()
			return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_APPLYING, status
		case *os_system_stager.Revision_RolledBack:
			status.Message = "update rolled back: running " + accepted
			status.Error = "trial " + u.RolledBack.MarshalString() + " failed"
			return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_FAILED, status
		}
		status.Message = "running " + accepted
		switch existing.GetUpdateState() {
		case s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_UNKNOWN, s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_IDLE:
			return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_IDLE, status
		}
		return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_UPDATED, status
	}
}

func deviceLauncherUpdateProjection(
	existing *s4wave_device.Device,
	info *spacewave_launcher.LauncherInfo,
	now time.Time,
) (s4wave_device.DeviceUpdateState, *s4wave_device.DeviceStatus) {
	ts := timestamppb.New(now)
	state := info.GetUpdateState()
	if state.GetPhase() == spacewave_launcher.UpdatePhase_UPDATE_PHASE_UNKNOWN {
		if existing.GetUpdateState() == s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_APPLYING {
			return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_UPDATED, &s4wave_device.DeviceStatus{
				Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
				Message:    "update applied",
				ObservedAt: ts,
			}
		}
		return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_IDLE, &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			Message:    "device session ready",
			ObservedAt: ts,
		}
	}

	switch state.GetPhase() {
	case spacewave_launcher.UpdatePhase_UPDATE_PHASE_DOWNLOADING:
		return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_STAGING, &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			Message:    deviceUpdateStatusMessage("staging update", state),
			ObservedAt: ts,
		}
	case spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED:
		return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_READY, &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			Message:    deviceUpdateStatusMessage("update ready", state),
			ObservedAt: ts,
		}
	case spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING:
		return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_APPLYING, &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			Message:    deviceUpdateStatusMessage("applying update", state),
			ObservedAt: ts,
		}
	case spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR:
		return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_FAILED, &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_DEGRADED,
			Message:    "update failed",
			Error:      state.GetErrorMessage(),
			ObservedAt: ts,
		}
	default:
		return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_IDLE, &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			Message:    "device session ready",
			ObservedAt: ts,
		}
	}
}

func deviceUpdateStatusMessage(prefix string, state *spacewave_launcher.UpdateState) string {
	if state.GetVersion() == "" {
		return prefix
	}
	return prefix + ": " + state.GetVersion()
}

func sameDeviceStatus(a, b *s4wave_device.DeviceStatus) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.GetLiveness() == b.GetLiveness() &&
		a.GetMessage() == b.GetMessage() &&
		a.GetError() == b.GetError()
}
