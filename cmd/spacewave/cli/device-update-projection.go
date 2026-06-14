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
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	"github.com/sirupsen/logrus"
)

func startDeviceLauncherUpdateProjection(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	b bus.Bus,
	invoker srpc.Invoker,
) {
	if b == nil || invoker == nil {
		return
	}
	client, err := buildSDKClientFromInvoker(ctx, invoker)
	if err != nil {
		if ctx.Err() == nil {
			le.WithError(err).Warn("device update projection unavailable")
		}
		return
	}
	go func() {
		defer client.close()
		if err := runDeviceLauncherUpdateProjection(ctx, le, statePath, b, client); err != nil && ctx.Err() == nil {
			le.WithError(err).Warn("device update projection stopped")
		}
	}()
}

func runDeviceLauncherUpdateProjection(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	b bus.Bus,
	client *sdkClient,
) error {
	invokers, _, invokerRef, err := bifrost_rpc.ExLookupRpcService(
		ctx,
		b,
		spacewave_launcher.SRPCLauncherServiceID,
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
		if err := projectDeviceLauncherInfo(ctx, statePath, client, info, time.Now()); err != nil {
			if stderrors.Is(err, world.ErrObjectNotFound) {
				le.WithError(err).Debug("device object not available for update projection")
				continue
			}
			le.WithError(err).Warn("failed to project launcher update state onto device")
		}
	}
}

func projectDeviceLauncherInfo(
	ctx context.Context,
	statePath string,
	client *sdkClient,
	info *spacewave_launcher.LauncherInfo,
	now time.Time,
) error {
	record, ok, err := deviceLauncherProjectionTarget(statePath)
	if err != nil || !ok {
		return err
	}
	spaceID, err := decodeDeviceResourceID(record.ResourceID)
	if err != nil {
		return err
	}

	sess, err := client.mountSession(ctx, record.SessionIndex)
	if err != nil {
		return err
	}
	defer sess.Release()

	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		return err
	}
	defer spaceCleanup()

	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		return err
	}
	defer engineCleanup()

	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	objState, found, err := tx.GetObject(ctx, record.DeviceObjectKey)
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
	next, changed, err := projectLauncherUpdateOntoDevice(existing, info, now)
	if err != nil || !changed {
		return err
	}

	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(next, true)
		return nil
	})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func deviceLauncherProjectionTarget(statePath string) (*deviceSetupRecord, bool, error) {
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

func projectLauncherUpdateOntoDevice(
	existing *s4wave_device.Device,
	info *spacewave_launcher.LauncherInfo,
	now time.Time,
) (*s4wave_device.Device, bool, error) {
	if existing == nil {
		return nil, false, errors.New("device state is required")
	}
	next := existing.CloneVT()
	updateState, status := deviceLauncherUpdateProjection(existing, info, now)

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

func deviceLauncherUpdateProjection(
	existing *s4wave_device.Device,
	info *spacewave_launcher.LauncherInfo,
	now time.Time,
) (s4wave_device.DeviceUpdateState, *s4wave_device.DeviceStatus) {
	ts := timestamppb.New(now)
	state := info.GetUpdateState()
	if state == nil || state.GetPhase() == spacewave_launcher.UpdatePhase_UpdatePhase_IDLE {
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
	case spacewave_launcher.UpdatePhase_UpdatePhase_DOWNLOADING:
		return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_STAGING, &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			Message:    deviceUpdateStatusMessage("staging update", state),
			ObservedAt: ts,
		}
	case spacewave_launcher.UpdatePhase_UpdatePhase_STAGED:
		return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_READY, &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			Message:    deviceUpdateStatusMessage("update ready", state),
			ObservedAt: ts,
		}
	case spacewave_launcher.UpdatePhase_UpdatePhase_APPLYING:
		return s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_APPLYING, &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			Message:    deviceUpdateStatusMessage("applying update", state),
			ObservedAt: ts,
		}
	case spacewave_launcher.UpdatePhase_UpdatePhase_ERROR:
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
