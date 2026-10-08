//go:build !js

package spacewave_cli

import (
	"context"
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
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	"github.com/sirupsen/logrus"
)

const (
	devicePolicyForgeWorkerCapabilityID = "forge-worker"
	devicePolicyRefPrefix               = "device-policy/"
)

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
	// Open the write transaction that verifies and writes together.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	// Verify the declared Worker object inside the same transaction that
	// writes the Device block, so verification and the capability write
	// commit or abort together.
	if fw := policy.GetForgeWorker(); fw != nil {
		if err := verifyForgeWorkerLink(ctx, tx, fw.GetWorkerObjectKey()); err != nil {
			return err
		}
	}

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
	policy *device_policy.DevicePolicy,
	now time.Time,
) (*s4wave_device.Device, bool, error) {
	// Copy policy capabilities onto the Device when they changed.
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
	// Build capabilities from the policy, keeping existing capability state.
	// Flowgraph node capabilities belong to the Flowgraph reconciler, so they
	// stay last in their existing order.
	existingByID := make(map[string]*s4wave_device.DeviceCapability, len(existing))
	var nodes []*s4wave_device.DeviceCapability
	out := make([]*s4wave_device.DeviceCapability, 0, len(existing)+2)
	for _, cap := range existing {
		if cap == nil {
			continue
		}
		id := strings.TrimSpace(cap.GetId())
		existingByID[id] = cap
		switch {
		case isDevicePolicyCapability(id, cap):
			// The policy recomputes its own capabilities below, which drops the
			// ones it wrote for a setting the policy no longer has.
		case s4wave_flowgraph.IsNodeCapabilityID(id):
			nodes = append(nodes, cap.CloneVT())
		default:
			out = append(out, cap.CloneVT())
		}
	}
	if fw := policy.GetForgeWorker(); fw != nil {
		out = append(out, computeForgeWorkerCapability(policy, fw, existingByID[devicePolicyForgeWorkerCapabilityID]))
	}
	return append(out, nodes...)
}

// computeForgeWorkerCapability authors or refreshes the forge-worker
// capability from the declared capacity envelope. The link always carries the
// policy-declared Worker object key and its forge/worker type id.
func computeForgeWorkerCapability(
	policy *device_policy.DevicePolicy,
	fw *device_policy.ForgeWorkerPolicy,
	existing *s4wave_device.DeviceCapability,
) *s4wave_device.DeviceCapability {
	state, detail := computeDevicePolicyCapabilityState("", existing)
	return &s4wave_device.DeviceCapability{
		Id:     devicePolicyForgeWorkerCapabilityID,
		Kind:   s4wave_device.DeviceCapabilityKindForgeWorker,
		Label:  "Forge Worker",
		State:  state,
		Detail: detail,
		Policy: computeDeviceCapabilityPolicy(policyRef(policy.GetRevision(), "forge-worker"), existing),
		Link: &s4wave_device.DeviceCapabilityLink{
			ObjectKey: fw.GetWorkerObjectKey(),
			TypeId:    forge_worker.WorkerTypeID,
		},
	}
}

func computeDeviceCapabilityPolicy(localRef string, existing *s4wave_device.DeviceCapability) *s4wave_device.DeviceCapabilityPolicy {
	// Copy grant fields from the existing capability policy.
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

// verifyForgeWorkerLink proves the declared Worker object exists and carries
// the forge/worker type quad. It runs inside the caller's transaction.
func verifyForgeWorkerLink(ctx context.Context, ws world.WorldState, workerObjectKey string) error {
	// Check the worker type and require it to belong to a Cluster.
	{
		_, objectState, err := forge_worker.LookupWorker(ctx, ws, workerObjectKey)
		world.ReleaseObjectState(objectState)
		if err != nil {
			return errors.Wrapf(err, "verify forge worker %q", workerObjectKey)
		}
	}
	if err := forge_worker.CheckWorkerType(ctx, ws, workerObjectKey); err != nil {
		return errors.Wrapf(err, "verify forge worker %q", workerObjectKey)
	}
	clusterKeys, err := forge_cluster.ListWorkerClusters(ctx, ws, workerObjectKey)
	if err != nil {
		return errors.Wrapf(err, "list clusters for Forge Worker %q", workerObjectKey)
	}
	if len(clusterKeys) == 0 {
		return errors.Errorf("Forge Worker %q is not assigned to a Cluster", workerObjectKey)
	}
	for _, clusterKey := range clusterKeys {
		if err := forge_cluster.CheckClusterType(ctx, ws, clusterKey); err != nil {
			return errors.Wrapf(err, "verify Cluster %q for Forge Worker %q", clusterKey, workerObjectKey)
		}
	}
	return nil
}

// isDevicePolicyCapability reports whether the policy projection owns the
// capability with the given ID: the forge-worker capability, and any capability
// it wrote earlier, which carries a local policy ref of its prefix.
func isDevicePolicyCapability(id string, capability *s4wave_device.DeviceCapability) bool {
	return id == devicePolicyForgeWorkerCapabilityID ||
		strings.HasPrefix(capability.GetPolicy().GetLocalPolicyRef(), devicePolicyRefPrefix)
}

func policyRef(revision uint64, suffix string) string {
	return devicePolicyRefPrefix + strconv.FormatUint(revision, 10) + "/" + suffix
}
