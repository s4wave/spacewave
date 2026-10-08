//go:build !js

package spacewave_cli

import (
	"context"
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
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	"github.com/sirupsen/logrus"
)

// devicePolicyRefPrefix is the local policy ref prefix of the capabilities the
// policy projection wrote earlier. The projection owns every capability that
// carries it and drops them.
const devicePolicyRefPrefix = "device-policy/"

// startDevicePolicyCapabilityProjection projects the local device policy into
// the Device object of the setup Space for the life of ctx.
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
		// Connect the SDK client the projection reads the Space through.
		client, err := buildSDKClientFromInvoker(ctx, invoker)
		if err != nil {
			if ctx.Err() == nil {
				le.WithError(err).Warn("device policy capability projection unavailable")
			}
			return
		}
		defer client.close()

		// Project each policy until ctx ends.
		if err := runDevicePolicyCapabilityProjection(ctx, le, statePath, client, store); err != nil && ctx.Err() == nil {
			le.WithError(err).Warn("device policy capability projection stopped")
		}
	}()
}

// runDevicePolicyCapabilityProjection projects every policy revision. A
// policy change abandons the pending projection of the previous one.
func runDevicePolicyCapabilityProjection(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	client *sdkClient,
	store *device_policy.PolicyStore,
) error {
	var last *device_policy.DevicePolicy
	for {
		// Wait for a policy the Space does not carry yet.
		policy, err := store.WaitChange(ctx, last)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		last = policy

		// Project it until it lands or the policy changes again.
		policyCtx, cancel := context.WithCancel(ctx)
		go func() {
			_, _ = store.WaitChange(policyCtx, policy)
			cancel()
		}()
		err = projectDevicePolicyWhenReadable(policyCtx, le, statePath, client, policy)
		cancel()
		if err != nil && policyCtx.Err() == nil {
			le.WithError(err).Warn("failed to project device policy capabilities")
		}
	}
}

// projectDevicePolicyWhenReadable projects policy into the Device object,
// retrying after each World change until the projection commits. A follower
// can start before its World holds the Device object or the blocks the
// projection reads; the next accepted World root may supply them.
func projectDevicePolicyWhenReadable(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	client *sdkClient,
	policy *device_policy.DevicePolicy,
) error {
	// Resolve the Device this daemon set up and hold its Space and World engine
	// across every attempt.
	record, ok, err := deviceLauncherProjectionTarget(statePath)
	if err != nil || !ok {
		return err
	}
	engine, release, err := mountDeviceWorld(ctx, client, record)
	if err != nil {
		return err
	}
	defer release()

	// Retry after each World change until the projection commits.
	for {
		// Attempt the projection against the current World root.
		seqno, err := engine.GetSeqno(ctx)
		if err != nil {
			return errors.Wrap(err, "get world seqno")
		}
		err = projectDevicePolicyCapabilities(ctx, engine, record, policy, time.Now())
		if err == nil || ctx.Err() != nil {
			return err
		}

		// Wait for the World to change before trying again.
		le.WithError(err).WithField("seqno", seqno).Debug("device policy projection waits for the next world change")
		if _, err := engine.WaitSeqno(ctx, seqno+1); err != nil {
			return err
		}
	}
}

// projectDevicePolicyCapabilities writes the capabilities of policy into the
// Device object of record in one transaction.
func projectDevicePolicyCapabilities(
	ctx context.Context,
	engine world.Engine,
	record *deviceSetupRecord,
	policy *device_policy.DevicePolicy,
	now time.Time,
) error {
	// Open the write transaction.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	// Read the Device object of this daemon.
	objState, found, err := tx.GetObject(ctx, record.DeviceObjectKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return errors.Wrap(err, "get device object")
	}
	if !found {
		return world.ErrObjectNotFound
	}

	// Check its identity and compute its projected capabilities.
	existing, err := readDeviceBlock(ctx, objState)
	if err != nil {
		return errors.Wrap(err, "read device block")
	}
	if existing.GetPeerId() != record.PeerID {
		return errors.New("device object peer_id does not match setup state")
	}
	next, changed, err := projectDevicePolicyOntoDevice(existing, policy, now)
	if err != nil || !changed {
		return err
	}

	// Write the Device block and commit.
	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(next, true)
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "write device block")
	}
	return errors.Wrap(tx.Commit(ctx), "commit")
}

func projectDevicePolicyOntoDevice(
	existing *s4wave_device.Device,
	_ *device_policy.DevicePolicy,
	now time.Time,
) (*s4wave_device.Device, bool, error) {
	// Copy policy capabilities onto the Device when they changed.
	if existing == nil {
		return nil, false, errors.New("device state is required")
	}
	next := existing.CloneVT()
	nextCaps := computeDevicePolicyCapabilities(existing.GetCapabilities())
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

func computeDevicePolicyCapabilities(existing []*s4wave_device.DeviceCapability) []*s4wave_device.DeviceCapability {
	// Drop the capabilities the policy wrote earlier. Flowgraph node
	// capabilities belong to the Flowgraph reconciler, so they stay last in
	// their existing order.
	var nodes []*s4wave_device.DeviceCapability
	out := make([]*s4wave_device.DeviceCapability, 0, len(existing))
	for _, cap := range existing {
		if cap == nil {
			continue
		}
		id := strings.TrimSpace(cap.GetId())
		switch {
		case isDevicePolicyCapability(cap):
			// The policy writes no capabilities now, so drop the ones it wrote earlier.
		case s4wave_flowgraph.IsNodeCapabilityID(id):
			nodes = append(nodes, cap.CloneVT())
		default:
			out = append(out, cap.CloneVT())
		}
	}
	return append(out, nodes...)
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

// isDevicePolicyCapability reports whether the policy projection owns the
// capability: it carries a local policy ref of the policy prefix.
func isDevicePolicyCapability(capability *s4wave_device.DeviceCapability) bool {
	return strings.HasPrefix(capability.GetPolicy().GetLocalPolicyRef(), devicePolicyRefPrefix)
}
