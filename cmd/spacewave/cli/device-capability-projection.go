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
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	"github.com/sirupsen/logrus"
)

const (
	devicePolicyRemoteShellCapabilityID   = "remote-shell"
	devicePolicyRemoteShellCapabilityKind = "remote-shell"
	devicePolicyCheckoutRootIDPrefix      = "checkout-root-"
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
	client, err := buildSDKClientFromInvoker(ctx, invoker)
	if err != nil {
		if ctx.Err() == nil {
			le.WithError(err).Warn("device policy capability projection unavailable")
		}
		return
	}
	go func() {
		defer client.close()
		if err := runDevicePolicyCapabilityProjection(ctx, le, statePath, client, store); err != nil && ctx.Err() == nil {
			le.WithError(err).Warn("device policy capability projection stopped")
		}
	}()
}

func runDevicePolicyCapabilityProjection(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	client *sdkClient,
	store *device_policy.PolicyStore,
) error {
	var last *device_policy.DevicePolicy
	for {
		policy, err := store.WaitChange(ctx, last)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := projectDevicePolicyCapabilities(ctx, statePath, client, policy, time.Now()); err != nil {
			if stderrors.Is(err, world.ErrObjectNotFound) {
				le.WithError(err).Debug("device object not available for policy projection")
				last = policy
				continue
			}
			le.WithError(err).Warn("failed to project device policy capabilities")
		}
		last = policy
	}
}

func projectDevicePolicyCapabilities(
	ctx context.Context,
	statePath string,
	client *sdkClient,
	policy *device_policy.DevicePolicy,
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
	next, changed, err := projectDevicePolicyOntoDevice(existing, policy, now)
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
	now time.Time,
) (*s4wave_device.Device, bool, error) {
	if existing == nil {
		return nil, false, errors.New("device state is required")
	}
	next := existing.CloneVT()
	nextCaps := computeDevicePolicyCapabilities(policy, existing.GetCapabilities())
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
) []*s4wave_device.DeviceCapability {
	existingByID := make(map[string]*s4wave_device.DeviceCapability, len(existing))
	out := make([]*s4wave_device.DeviceCapability, 0, len(existing)+len(policy.GetCheckoutRoot())+1)
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
	return out
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
	return id == devicePolicyRemoteShellCapabilityID || strings.HasPrefix(id, devicePolicyCheckoutRootIDPrefix)
}

func policyRef(revision uint64, suffix string) string {
	return devicePolicyRefPrefix + strconv.FormatUint(revision, 10) + "/" + suffix
}
