//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/core/device/sensor"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/sirupsen/logrus"
)

const (
	devicePolicyRemoteShellCapabilityID   = "remote-shell"
	devicePolicyRemoteShellCapabilityKind = "remote-shell"
	devicePolicyCheckoutRootIDPrefix      = "checkout-root-"
	devicePolicySensorCapabilityIDPrefix  = "sensor-"
	devicePolicyRefPrefix                 = "device-policy/"
)

func startDevicePolicyCapabilityProjection(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	b bus.Bus,
	invoker srpc.Invoker,
	store *device_policy.PolicyStore,
) {
	if b == nil || invoker == nil || store == nil {
		return
	}
	go func() {
		client, err := buildSDKClientFromInvoker(ctx, invoker)
		if err != nil {
			if ctx.Err() == nil {
				le.WithError(err).Warn("device policy capability projection unavailable")
			}
			return
		}
		defer client.close()
		if err := runDevicePolicyCapabilityProjection(ctx, le, statePath, client, store); err != nil && ctx.Err() == nil {
			le.WithError(err).Warn("device policy capability projection stopped")
		}
	}()
}

// devicePolicyUpdate carries one policy change to the projection loop.
type devicePolicyUpdate struct {
	policy *device_policy.DevicePolicy
	err    error
}

// watchDevicePolicyUpdates feeds every policy change into updates until the
// context ends or the store fails.
func watchDevicePolicyUpdates(
	ctx context.Context,
	store *device_policy.PolicyStore,
	updates chan<- devicePolicyUpdate,
) {
	defer close(updates)
	var last *device_policy.DevicePolicy
	for {
		policy, err := store.WaitChange(ctx, last)
		if ctx.Err() != nil {
			return
		}
		sendErr := func(u devicePolicyUpdate) bool {
			select {
			case updates <- u:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if err != nil {
			sendErr(devicePolicyUpdate{err: err})
			return
		}
		if !sendErr(devicePolicyUpdate{policy: policy}) {
			return
		}
		last = policy
	}
}

// deviceSensorRun holds the mounts and sensor manager for one ready Device.
// The run lives while the Device session and Space engine stay available;
// release cancels the run context, joins the adapters, and tears down mounts
// through their own lifecycles.
type deviceSensorRun struct {
	le      *logrus.Entry
	record  *deviceSetupRecord
	engine  *sdk_engine.SDKEngine
	manager *sensor.Manager

	ctx    context.Context
	cancel context.CancelFunc

	cleanups []func()
}

func (r *deviceSensorRun) release() {
	r.cancel()
	if r.manager != nil {
		r.manager.Close()
	}
	for i := len(r.cleanups) - 1; i >= 0; i-- {
		r.cleanups[i]()
	}
	r.cleanups = nil
}

// mountDeviceSensorRun mounts the Device session, Space, and World engine and
// constructs the sensor manager. It returns nil without error when the Device
// setup target is not ready yet.
func mountDeviceSensorRun(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	client *sdkClient,
) (*deviceSensorRun, error) {
	record, ok, err := deviceLauncherProjectionTarget(statePath)
	if err != nil || !ok {
		return nil, err
	}
	spaceID, err := decodeDeviceResourceID(record.ResourceID)
	if err != nil {
		return nil, err
	}

	runCtx, runCancel := context.WithCancel(ctx)
	run := &deviceSensorRun{le: le, record: record, ctx: runCtx, cancel: runCancel}
	fail := func(err error) (*deviceSensorRun, error) {
		run.release()
		return nil, err
	}

	sess, err := client.mountSession(ctx, record.SessionIndex)
	if err != nil {
		return fail(err)
	}
	run.cleanups = append(run.cleanups, sess.Release)

	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		return fail(err)
	}
	run.cleanups = append(run.cleanups, spaceCleanup)

	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		return fail(err)
	}
	run.engine = engine
	run.cleanups = append(run.cleanups, engineCleanup)

	run.manager = sensor.NewManager(
		le.WithField("device_object_key", record.DeviceObjectKey),
		engine,
		record.DeviceObjectKey,
		nil,
	)
	return run, nil
}

// reconcile starts and stops sensor adapters for the current policy. Adapter
// contexts derive from the run context so release joins every adapter.
func (r *deviceSensorRun) reconcile(policy *device_policy.DevicePolicy) {
	r.manager.Reconcile(r.ctx, policy.GetSensorEndpoint())
}

// sensorStatusLookup reads live adapter status for capability projection.
func (r *deviceSensorRun) sensorStatusLookup() func(string) (sensor.Status, bool) {
	return r.manager.Status
}

func runDevicePolicyCapabilityProjection(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	client *sdkClient,
	store *device_policy.PolicyStore,
) error {
	updates := make(chan devicePolicyUpdate)
	go watchDevicePolicyUpdates(ctx, store, updates)

	var run *deviceSensorRun
	defer func() {
		if run != nil {
			run.release()
		}
	}()
	for {
		var policy *device_policy.DevicePolicy
		if run == nil {
			u, ok := <-updates
			if !ok || u.err != nil {
				if ctx.Err() != nil || !ok {
					return nil
				}
				return u.err
			}
			policy = u.policy
		} else {
			select {
			case u, ok := <-updates:
				if !ok {
					return nil
				}
				if u.err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return u.err
				}
				policy = u.policy
			case <-run.manager.Changed():
				policy = store.Snapshot()
			}
		}

		if run == nil {
			mounted, err := mountDeviceSensorRun(ctx, le, statePath, client)
			if err != nil {
				le.WithError(err).Warn("failed to mount device policy projection")
				continue
			}
			if mounted == nil {
				continue
			}
			run = mounted
		}

		// Adapters start only after session and engine readiness above.
		run.reconcile(policy)
		if err := projectDevicePolicyCapabilities(ctx, run, policy, time.Now()); err != nil {
			if stderrors.Is(err, world.ErrObjectNotFound) {
				le.WithError(err).Debug("device object not available for policy projection")
				run.release()
				run = nil
				continue
			}
			le.WithError(err).Warn("failed to project device policy capabilities")
		}
	}
}

func projectDevicePolicyCapabilities(
	ctx context.Context,
	run *deviceSensorRun,
	policy *device_policy.DevicePolicy,
	now time.Time,
) error {
	tx, err := run.engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	objState, found, err := tx.GetObject(ctx, run.record.DeviceObjectKey)
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
	if existing.GetPeerId() != run.record.PeerID {
		return errors.New("device object peer_id does not match setup state")
	}
	next, changed, err := projectDevicePolicyOntoDevice(
		existing,
		policy,
		run.record.DeviceObjectKey,
		run.sensorStatusLookup(),
		now,
	)
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

func projectDevicePolicyOntoDevice(
	existing *s4wave_device.Device,
	policy *device_policy.DevicePolicy,
	deviceObjectKey string,
	sensorStatus func(string) (sensor.Status, bool),
	now time.Time,
) (*s4wave_device.Device, bool, error) {
	if existing == nil {
		return nil, false, errors.New("device state is required")
	}
	next := existing.CloneVT()
	nextCaps := computeDevicePolicyCapabilities(policy, existing.GetCapabilities(), deviceObjectKey, sensorStatus)
	if sameDeviceCapabilities(nextCaps, existing.GetCapabilities()) {
		return next, false, nil
	}
	next.Capabilities = nextCaps
	next.UpdatedAt = timestamppb.New(now)
	if err := next.Validate(); err != nil {
		return nil, false, err
	}
	return next, true, nil
}

func computeDevicePolicyCapabilities(
	policy *device_policy.DevicePolicy,
	existing []*s4wave_device.DeviceCapability,
	deviceObjectKey string,
	sensorStatus func(string) (sensor.Status, bool),
) []*s4wave_device.DeviceCapability {
	existingByID := make(map[string]*s4wave_device.DeviceCapability, len(existing))
	out := make([]*s4wave_device.DeviceCapability, 0, len(existing)+len(policy.GetCheckoutRoot())+len(policy.GetSensorEndpoint())+1)
	for _, cap := range existing {
		if cap == nil {
			continue
		}
		id := strings.TrimSpace(cap.GetId())
		existingByID[id] = cap
		if isDevicePolicyCapabilityID(id) {
			continue
		}
		out = append(out, cap.CloneVT())
	}
	if policy.GetRemoteShell().GetEnabled() {
		out = append(out, computeRemoteShellCapability(policy, existingByID[devicePolicyRemoteShellCapabilityID]))
	}
	for _, root := range policy.GetCheckoutRoot() {
		if root == nil {
			continue
		}
		id := devicePolicyCheckoutRootIDPrefix + strings.TrimSpace(root.GetName())
		out = append(out, computeCheckoutRootCapability(policy, root, existingByID[id]))
	}
	for _, endpoint := range policy.GetSensorEndpoint() {
		if endpoint == nil || !endpoint.GetEnabled() {
			continue
		}
		id := strings.TrimSpace(endpoint.GetId())
		var status *sensor.Status
		if sensorStatus != nil {
			if live, ok := sensorStatus(id); ok {
				status = &live
			}
		}
		out = append(out, computeSensorCapability(
			policy,
			endpoint,
			deviceObjectKey,
			existingByID[devicePolicySensorCapabilityIDPrefix+id],
			status,
		))
	}
	return out
}

func computeSensorCapability(
	policy *device_policy.DevicePolicy,
	endpoint *device_policy.SensorEndpointPolicy,
	deviceObjectKey string,
	existing *s4wave_device.DeviceCapability,
	status *sensor.Status,
) *s4wave_device.DeviceCapability {
	id := strings.TrimSpace(endpoint.GetId())
	state, detail := computeSensorCapabilityState(status, existing)
	return &s4wave_device.DeviceCapability{
		Id:     devicePolicySensorCapabilityIDPrefix + id,
		Kind:   s4wave_device.DeviceCapabilityKindSensor,
		Label:  sensor.EndpointLabel(id) + " sensor",
		State:  state,
		Detail: detail,
		Policy: computeDeviceCapabilityPolicy(policyRef(policy.GetRevision(), "sensor/"+id), existing),
		Link: &s4wave_device.DeviceCapabilityLink{
			ObjectKey: sensor.ObjectKey(deviceObjectKey, id),
			TypeId:    s4wave_device.SensorTypeID,
		},
	}
}

// computeSensorCapabilityState maps live connection state onto the projected
// capability state. A linked Sensor stays visible during transient disconnect
// with explicit degraded or offline detail.
func computeSensorCapabilityState(
	status *sensor.Status,
	existing *s4wave_device.DeviceCapability,
) (s4wave_device.DeviceCapabilityState, string) {
	if existing != nil && existing.GetPolicy().GetGrantState() == s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_BLOCKED {
		if existing.GetDetail() != "" {
			return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED, existing.GetDetail()
		}
		return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED, "blocked by Space grant"
	}
	if status == nil {
		return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE, "waiting for device session"
	}
	switch status.ConnectionState {
	case s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_CONNECTED:
		return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE, ""
	case s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_CONNECTING:
		return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE, "connecting"
	case s4wave_device.SensorConnectionState_SENSOR_CONNECTION_STATE_DEGRADED:
		return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE, joinSensorDetail("degraded", status.LastError)
	default:
		return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE, joinSensorDetail("offline", status.LastError)
	}
}

func joinSensorDetail(state, lastError string) string {
	if strings.TrimSpace(lastError) == "" {
		return state
	}
	return state + ": " + lastError
}

func computeRemoteShellCapability(
	policy *device_policy.DevicePolicy,
	existing *s4wave_device.DeviceCapability,
) *s4wave_device.DeviceCapability {
	state, detail := computeDevicePolicyCapabilityState(policy.GetRemoteShell().GetDetail(), existing)
	return &s4wave_device.DeviceCapability{
		Id:     devicePolicyRemoteShellCapabilityID,
		Kind:   devicePolicyRemoteShellCapabilityKind,
		Label:  "Remote Shell",
		State:  state,
		Detail: detail,
		Policy: computeDeviceCapabilityPolicy(policyRef(policy.GetRevision(), "remote-shell"), existing),
	}
}

func computeCheckoutRootCapability(
	policy *device_policy.DevicePolicy,
	root *device_policy.CheckoutRootPolicy,
	existing *s4wave_device.DeviceCapability,
) *s4wave_device.DeviceCapability {
	name := strings.TrimSpace(root.GetName())
	access := root.GetAccess()
	state, detail := computeDevicePolicyCapabilityState("", existing)
	cap := &s4wave_device.DeviceCapability{
		Id:     devicePolicyCheckoutRootIDPrefix + name,
		Kind:   s4wave_device.DeviceCapabilityKindFilesystem,
		Label:  name + " checkout",
		State:  state,
		Detail: detail,
		Policy: computeDeviceCapabilityPolicy(policyRef(policy.GetRevision(), "checkout-root/"+name), existing),
		CheckoutRoot: &s4wave_device.DeviceCheckoutRootCapability{
			Name:           name,
			DisplayPath:    strings.TrimSpace(root.GetLocalPath()),
			SelectionRef:   policyRef(policy.GetRevision(), "checkout-root/"+name),
			Access:         access,
			ReadAvailable:  true,
			WriteAvailable: access == s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE,
		},
	}
	if existing.GetLink() != nil {
		cap.Link = existing.GetLink().CloneVT()
	}
	return cap
}

func computeDeviceCapabilityPolicy(localRef string, existing *s4wave_device.DeviceCapability) *s4wave_device.DeviceCapabilityPolicy {
	policy := &s4wave_device.DeviceCapabilityPolicy{
		LocalPolicyRef: localRef,
		LocalState:     s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
		GrantState:     s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
	}
	if existing == nil || existing.GetPolicy() == nil {
		return policy
	}
	existingPolicy := existing.GetPolicy()
	policy.GrantPolicyRef = existingPolicy.GetGrantPolicyRef()
	if existingPolicy.GetGrantState() != s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_UNKNOWN {
		policy.GrantState = existingPolicy.GetGrantState()
	}
	return policy
}

func computeDevicePolicyCapabilityState(
	detail string,
	existing *s4wave_device.DeviceCapability,
) (s4wave_device.DeviceCapabilityState, string) {
	if existing != nil && existing.GetPolicy().GetGrantState() == s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_BLOCKED {
		if existing.GetDetail() != "" {
			return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED, existing.GetDetail()
		}
		return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED, "blocked by Space grant"
	}
	if existing != nil && existing.GetState() == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE {
		return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE, detail
	}
	return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE, detail
}

func sameDeviceCapabilities(a, b []*s4wave_device.DeviceCapability) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].EqualVT(b[i]) {
			return false
		}
	}
	return true
}

func isDevicePolicyCapabilityID(id string) bool {
	return id == devicePolicyRemoteShellCapabilityID ||
		strings.HasPrefix(id, devicePolicyCheckoutRootIDPrefix) ||
		strings.HasPrefix(id, devicePolicySensorCapabilityIDPrefix)
}

func policyRef(revision uint64, suffix string) string {
	return devicePolicyRefPrefix + strconv.FormatUint(revision, 10) + "/" + suffix
}
