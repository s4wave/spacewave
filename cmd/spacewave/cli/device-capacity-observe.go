//go:build !js

package spacewave_cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	device_policy "github.com/s4wave/spacewave/core/device/policy"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
)

const (
	// capacityReservationLease is the reservation lease used by the daemon's
	// admission owner.
	capacityReservationLease = 10 * time.Minute
	// capacityOwnerLease is the owner claim lease; the renewal ticker runs at
	// a fraction of it so the claim outlives the interval.
	capacityOwnerLease  = time.Minute
	capacityRenewPeriod = capacityOwnerLease / 3
)

// startDeviceCapacityObserver claims, observes, and drains the declared
// forge-worker capacity envelope through the merged owner-state admission
// APIs. It follows policy changes and renews the claim while the daemon runs.
func startDeviceCapacityObserver(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	invoker srpc.Invoker,
	store *device_policy.PolicyStore,
) {
	if invoker == nil || store == nil {
		return
	}
	go func() {
		client, err := buildSDKClientFromInvoker(ctx, invoker)
		if err != nil {
			if ctx.Err() == nil {
				le.WithError(err).Warn("device capacity observer unavailable")
			}
			return
		}
		defer client.close()
		if err := runDeviceCapacityObserver(ctx, le, statePath, client, store); err != nil && ctx.Err() == nil {
			le.WithError(err).Warn("device capacity observer stopped")
		}
	}()
}

// runDeviceCapacityObserver reacts to every accepted policy revision and
// renews the claim between revisions. The ticker is lease-driven, not a state
// poll: only the claim deadline forces work between policy changes.
func runDeviceCapacityObserver(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	client *sdkClient,
	store *device_policy.PolicyStore,
) error {
	enrollmentCleanup, err := restoreLocalDeviceEnrollment(ctx, statePath, client)
	if err != nil {
		return errors.Wrap(err, "restore local Device enrollment")
	}
	if enrollmentCleanup != nil {
		defer enrollmentCleanup()
	}
	claimID, err := newClaimID()
	if err != nil {
		return errors.Wrap(err, "generate claim id")
	}

	type policyUpdate struct {
		policy *device_policy.DevicePolicy
		err    error
	}
	watchCtx, stopWatch := context.WithCancel(ctx)
	updates := make(chan policyUpdate, 1)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		var last *device_policy.DevicePolicy
		for {
			policy, err := store.WaitChange(watchCtx, last)
			select {
			case updates <- policyUpdate{policy: policy, err: err}:
			case <-watchCtx.Done():
				return
			}
			if err != nil {
				return
			}
			last = policy
		}
	}()
	defer func() {
		stopWatch()
		<-watchDone
	}()

	var admission *forge_runtime.WorldRuntimeAdmission
	var ref forge_runtime.WorkerClaimRef
	var cleanup func()
	closeAdmission := func() {
		if cleanup != nil {
			cleanup()
		}
		admission = nil
		ref = forge_runtime.WorkerClaimRef{}
		cleanup = nil
	}
	defer closeAdmission()

	var current *device_policy.DevicePolicy
	var havePolicy bool
	var applyNeeded bool
	ticker := time.NewTicker(capacityRenewPeriod)
	defer ticker.Stop()
	for {
		if applyNeeded && havePolicy {
			applyNeeded = false
			if admission == nil {
				admission, ref, cleanup, err = openDeviceCapacityAdmission(ctx, statePath, client, claimID)
				if err != nil {
					le.WithError(err).Warn("failed to open declared capacity target")
					closeAdmission()
				} else if admission == nil {
					closeAdmission()
				}
			}
			if admission != nil {
				var declared *device_policy.ForgeWorkerPolicy
				if current != nil {
					declared = current.GetForgeWorker()
				}
				if err := applyDeclaredCapacity(ctx, le, admission, ref, declared); err != nil {
					le.WithError(err).Warn("failed to observe declared capacity")
					closeAdmission()
				}
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case update := <-updates:
			if update.err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return update.err
			}
			current = update.policy
			havePolicy = true
			applyNeeded = true
		case <-ticker.C:
			if admission == nil {
				applyNeeded = true
				continue
			}
			if err := renewOwnedCapacityOnceWithAdmission(ctx, admission, ref.DeviceObjectKey, ref); err != nil {
				if ctx.Err() == nil {
					le.WithError(err).Warn("capacity claim renewal failed")
				}
				closeAdmission()
				applyNeeded = true
			}
		}
	}
}

// restoreLocalDeviceEnrollment reopens the persisted local completion on each
// daemon start. This reasserts the invite authority as a desired signaling
// peer without replaying the one-use invite.
func restoreLocalDeviceEnrollment(ctx context.Context, statePath string, client *sdkClient) (func(), error) {
	record, ok, err := deviceLauncherProjectionTarget(statePath)
	if err != nil || !ok || !strings.HasPrefix(record.Completion, deviceLocalCompletionPrefix) {
		return nil, err
	}
	sess, err := client.mountSession(ctx, record.SessionIndex)
	if err != nil {
		return nil, err
	}
	updated, err := openLocalDeviceSession(ctx, client, statePath, record)
	if err != nil {
		sess.Release()
		return nil, err
	}
	if err := writeDeviceSetupRecord(statePath, updated); err != nil {
		sess.Release()
		return nil, err
	}
	return sess.Release, nil
}

// openDeviceCapacityAdmission mounts the Device session, Space, and World
// engine for the daemon lifetime. Retaining these resources keeps the session
// transport and its WebRTC reconnection loop alive between lease renewals.
func openDeviceCapacityAdmission(
	ctx context.Context,
	statePath string,
	client *sdkClient,
	claimID string,
) (*forge_runtime.WorldRuntimeAdmission, forge_runtime.WorkerClaimRef, func(), error) {
	record, ok, err := deviceLauncherProjectionTarget(statePath)
	if err != nil || !ok {
		return nil, forge_runtime.WorkerClaimRef{}, nil, err
	}
	sess, err := client.mountSession(ctx, record.SessionIndex)
	if err != nil {
		return nil, forge_runtime.WorkerClaimRef{}, nil, err
	}
	spaceID, err := decodeDeviceResourceID(record.ResourceID)
	if err != nil {
		sess.Release()
		return nil, forge_runtime.WorkerClaimRef{}, nil, err
	}
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		sess.Release()
		return nil, forge_runtime.WorkerClaimRef{}, nil, err
	}
	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		spaceCleanup()
		sess.Release()
		return nil, forge_runtime.WorkerClaimRef{}, nil, err
	}
	cleanup := func() {
		engineCleanup()
		spaceCleanup()
		sess.Release()
	}
	admission := forge_runtime.NewWorldRuntimeAdmission(engine, nil, capacityReservationLease, capacityOwnerLease)
	ref := forge_runtime.WorkerClaimRef{DeviceObjectKey: record.DeviceObjectKey, ClaimID: claimID}
	return admission, ref, cleanup, nil
}

// applyDeclaredCapacity claims every owned record, observes the declared key
// with the declared totals, and drains any other owned record. A nil
// declaration drains everything owned and never reactivates.
func applyDeclaredCapacity(
	ctx context.Context,
	le *logrus.Entry,
	admission *forge_runtime.WorldRuntimeAdmission,
	ref forge_runtime.WorkerClaimRef,
	declared *device_policy.ForgeWorkerPolicy,
) error {
	owned, err := admission.ScanOwnedCapacity(ctx, ref.DeviceObjectKey)
	if err != nil {
		return errors.Wrap(err, "scan owned capacity")
	}
	declaredKey := ""
	if declared != nil {
		declaredKey = declared.GetWorkerObjectKey()
	}
	handled := make(map[string]bool, len(owned)+1)
	for _, oc := range owned {
		key := oc.WorkerObjectKey
		handled[key] = true
		capacity, err := admission.ClaimWorkerCapacity(ctx, key, ref)
		if err != nil {
			return errors.Wrap(err, "claim owned worker capacity")
		}
		if key == declaredKey {
			if _, err := admission.ObserveWorker(ctx, key, ref, capacity.OwnerEpoch,
				declared.GetMilliCpu(), declared.GetMemoryBytes(), declared.GetBackends()); err != nil {
				return errors.Wrap(err, "observe declared worker capacity")
			}
			continue
		}
		// Stale owned key or removal: drain with empty backends and complete
		// when terminal. Completion failure means reservations are still
		// live; the next cycle retries.
		if _, err := admission.BeginDrainCapacity(ctx, key, ref, capacity.OwnerEpoch); err != nil {
			return errors.Wrap(err, "begin stale capacity drain")
		}
		if err := admission.CompleteDrainCapacity(ctx, key, ref, capacity.OwnerEpoch); err != nil {
			le.WithError(err).Debug("drained capacity retains live reservations")
		}
	}
	if declaredKey == "" || handled[declaredKey] {
		return nil
	}
	capacity, err := admission.ClaimWorkerCapacity(ctx, declaredKey, ref)
	if err != nil {
		return errors.Wrap(err, "claim declared worker capacity")
	}
	_, err = admission.ObserveWorker(ctx, declaredKey, ref, capacity.OwnerEpoch,
		declared.GetMilliCpu(), declared.GetMemoryBytes(), declared.GetBackends())
	return errors.Wrap(err, "observe declared worker capacity")
}

// renewOwnedCapacityOnceWithAdmission renews every record the Device owns
// through the daemon-lifetime World engine mount.
func renewOwnedCapacityOnceWithAdmission(
	ctx context.Context,
	admission *forge_runtime.WorldRuntimeAdmission,
	deviceObjectKey string,
	ref forge_runtime.WorkerClaimRef,
) error {
	owned, err := admission.ScanOwnedCapacity(ctx, deviceObjectKey)
	if err != nil {
		return err
	}
	for _, oc := range owned {
		if _, err := admission.RenewWorkerClaim(ctx, oc.WorkerObjectKey, ref); err != nil {
			return err
		}
	}
	return nil
}

// newClaimID generates this daemon invocation's claim identifier.
func newClaimID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
