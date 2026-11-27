package provider_local

import (
	"context"
	"math"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_backup "github.com/s4wave/spacewave/core/space/backup"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/core/space/world/objecttypes"
	"github.com/s4wave/spacewave/db/block"
	packcopy_world "github.com/s4wave/spacewave/db/block/packcopy/world"
	block_store_gdrive "github.com/s4wave/spacewave/db/block/store/gdrive"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/crypto"
)

// SpaceBackup owns one device-local backup's pass and watched state. The
// account's lifetime owns it, so disconnecting a command does not stop a copy.
type SpaceBackup struct {
	// notify wakes the account's registry watch after this backup changes.
	notify func()
	// state is the public, credential-free watched state.
	state *ccontainer.CContainer[*account_settings.SpaceBackupState]
	// routine restarts when the saved configuration changes.
	routine *routine.StateRoutineContainer[*account_settings.SpaceBackup]
	// bcast guards requests, settled and conf, and wakes the scheduler.
	bcast broadcast.Broadcast
	// requested is the newest requested or automatic pass sequence.
	requested uint64
	// settled is the newest terminal pass sequence.
	settled uint64
	// manualRequest preserves an explicit request if automatic work is disabled mid-pass.
	manualRequest bool
	// conf is the newest committed device-local configuration.
	conf *account_settings.SpaceBackup
}

// newSpaceBackup creates an idle routine without enabling automatic work.
func newSpaceBackup(
	conf *account_settings.SpaceBackup,
	initial *account_settings.SpaceBackupState,
	notify func(),
	runMounted func(context.Context, *account_settings.SpaceBackup, *SpaceBackup) error,
) *SpaceBackup {
	// The generated oneof is the sole public progress state.
	if initial == nil {
		initial = &account_settings.SpaceBackupState{State: &account_settings.SpaceBackupState_Idle{Idle: &account_settings.SpaceBackupIdle{}}}
	}
	initial.Backup = conf.CloneVT()
	b := &SpaceBackup{notify: notify, conf: conf.CloneVT(), requested: initial.GetRunSequence(), settled: initial.GetRunSequence(),
		state: ccontainer.NewCContainerVT(initial)}
	b.routine = routine.NewStateRoutineContainerVT[*account_settings.SpaceBackup](routine.WithRetry(providerBackoff))
	b.routine.SetStateRoutine(func(ctx context.Context, conf *account_settings.SpaceBackup) error {
		return b.execute(ctx, conf, runMounted)
	})
	return b
}

// configure publishes saved configuration and restarts its scheduler if changed.
func (b *SpaceBackup) configure(ctx context.Context, conf *account_settings.SpaceBackup) {
	// Preserve the progress state while showing the new committed configuration.
	b.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Stop only an automatic request when its approval is removed.
		b.conf = conf.CloneVT()
		state := b.state.GetValue().CloneVT()
		state.Backup = conf.CloneVT()
		if !conf.GetAutomatic() && !b.manualRequest && b.requested > b.settled {
			b.settled = b.requested
			state.State = &account_settings.SpaceBackupState_Idle{Idle: &account_settings.SpaceBackupIdle{}}
		}
		b.state.SetValue(state)
		broadcast()
	})
	b.routine.SetState(conf.CloneVT())
	b.routine.SetContext(ctx, true)
	b.notify()
}

// request joins an active pass or requests a new one and returns its sequence.
func (b *SpaceBackup) request(manual bool) uint64 {
	// Publish the request before waking the actor so a watcher cannot see an old terminal result.
	var sequence uint64
	b.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if b.requested == b.settled {
			b.requested++
			b.manualRequest = manual
			b.state.SetValue(&account_settings.SpaceBackupState{Backup: b.conf.CloneVT(), RunSequence: b.requested,
				State: &account_settings.SpaceBackupState_Copying{Copying: &account_settings.SpaceBackupProgress{}}})
		} else if manual {
			b.manualRequest = true
		}
		sequence = b.requested
		broadcast()
	})
	b.notify()
	return sequence
}

// currentState returns the immutable state snapshot used by registry watches.
func (b *SpaceBackup) currentState() *account_settings.SpaceBackupState {
	return b.state.GetValue()
}

// waitState blocks until the backup state differs from the caller's snapshot.
func (b *SpaceBackup) waitState(ctx context.Context, previous *account_settings.SpaceBackupState) (*account_settings.SpaceBackupState, error) {
	return b.state.WaitValueChange(ctx, previous, nil)
}

// pendingRequest captures the request sequence and its next change together.
func (b *SpaceBackup) pendingRequest() (sequence uint64, pending bool, changed <-chan struct{}) {
	b.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		sequence, pending, changed = b.requested, b.requested > b.settled, getWaitCh()
	})
	return sequence, pending, changed
}

// publish updates the watched state and settles this pass before another request.
func (b *SpaceBackup) publish(state *account_settings.SpaceBackupState, terminal bool) {
	// A cancelled old routine cannot replace a newer pass's progress.
	b.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if state.GetRunSequence() != b.requested || !terminal && b.requested == b.settled {
			return
		}
		state.Backup = b.conf.CloneVT()
		if terminal {
			b.settled = state.GetRunSequence()
		}
		b.state.SetValue(state)
	})
	b.notify()
}

// execute holds a read-only mounted Space and watches its checkpoint notifications.
func (b *SpaceBackup) execute(
	ctx context.Context,
	conf *account_settings.SpaceBackup,
	runMounted func(context.Context, *account_settings.SpaceBackup, *SpaceBackup) error,
) (runErr error) {
	// Settle a request if startup failed, without logging raw transport credentials.
	defer func() {
		if runErr != nil && ctx.Err() == nil {
			sequence, _, _ := b.pendingRequest()
			b.publish(&account_settings.SpaceBackupState{RunSequence: sequence, State: &account_settings.SpaceBackupState_Failed{
				Failed: &account_settings.SpaceBackupFailure{Message: "backup could not open its local Space; retry after it is available"},
			}}, true)
		}
	}()
	// With automatic off, keep no source mount or pass alive between requests.
	for {
		for !conf.GetAutomatic() {
			_, needed, changed := b.pendingRequest()
			if needed {
				break
			}
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case <-changed:
			}
		}
		if err := runMounted(ctx, conf, b); err != nil {
			return err
		}
	}
}

// runMountedBackup owns the read-only source mount for a requested or automatic pass.
func (a *ProviderAccount) runMountedBackup(ctx context.Context, conf *account_settings.SpaceBackup, b *SpaceBackup) error {
	// Resolve the catalog's full reference; a block store ID is not a Space ID.
	blockStoreID := a.lookupSharedObjectBlockStoreID(conf.GetSpaceId())
	if blockStoreID == "" {
		return errors.New("backup Space is not in this account's catalog")
	}
	ref := sobject.NewSharedObjectRef(a.GetProviderID(), a.GetAccountID(), conf.GetSpaceId(), blockStoreID)
	mounted, mountRef, err := space.ExMountSpaceSoBody(ctx, a.t.p.b, ref, false, nil)
	if err != nil {
		return err
	}
	defer mountRef.Release()
	body := mounted.GetSharedObjectBody()

	// Access the serving engine directly, so local pins never become World operations.
	return world.AccessEngine(ctx, body.GetWorldEngine(), func(ctx context.Context, engine world.Engine) error {
		// Access only the engine's device-local metadata and ciphertext source.
		local, ok := engine.(*space_world.Engine)
		if !ok {
			return errors.New("backup needs a locally mounted SharedObject World")
		}
		store, release, err := local.AccessBackupStateStore(ctx)
		if err != nil {
			return err
		}
		defer release()
		return a.scheduleBackup(ctx, conf, b, local, body.GetSharedObject().GetBlockStore(), store)
	})
}

// scheduleBackup serializes manual requests and checkpoint-driven automatic passes.
func (a *ProviderAccount) scheduleBackup(
	ctx context.Context, conf *account_settings.SpaceBackup, b *SpaceBackup,
	engine *space_world.Engine, source block.StoreOps, local kvtx.Store,
) error {
	// A failed quota or sign-in needs an owner action; an upload limit has a deadline.
	var attempted *account_settings.SpaceBackupSnapshot
	var lastPass, retryAt time.Time
	var ownerAction bool

	// Convert the saved interval only within the duration's representable range.
	seconds := conf.GetIntervalSeconds()
	if seconds > uint64(math.MaxInt64/int64(time.Second)) {
		return errors.New("backup interval is too long")
	}
	interval := time.Duration(seconds) * time.Second

	// A restart or enabling automatic work still respects the durable last publication.
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return err
	}
	backend := settings.FindStorageBackend(conf.GetStorageBackendId())
	if backend.GetGdrive() == nil {
		return errors.New("Drive backend no longer exists; configure a new backup")
	}

	// Respect this destination's last durable publication after a restart.
	destination := backend.GetId() + "/" + backend.GetGdrive().GetRootFolderId() + "/" + conf.GetSpaceId() + "/"
	previous, err := space_backup.ReadPublished(ctx, kvtx_prefixer.NewPrefixer(local, space_backup.Namespace(destination)))
	if err != nil {
		return err
	}
	if previous != nil {
		lastPass = time.Unix(previous.GetPublishedUnixSeconds(), 0)
	}

	// Preserve a known upload deadline and owner-action waits across process restarts.
	initial := b.currentState()
	ownerAction = initial.GetWaitingQuota() != nil || initial.GetWaitingSignIn() != nil
	if wait := initial.GetWaitingUploadLimit(); wait != nil {
		retryAt = time.Unix(wait.GetResumeUnixSeconds(), 0)
	}
	for {
		// Capture both sources' state with their own lossless notifications.
		snapshot, checkpointChanged := engine.BackupSnapshot()
		requested, manual, changed := b.pendingRequest()
		now := time.Now()
		due := lastPass.Add(interval)
		if retryAt.After(due) {
			due = retryAt
		}
		newCheckpoint := snapshot != nil && !snapshot.EqualVT(attempted)
		retryLimit := !retryAt.IsZero() && !now.Before(retryAt)
		automatic := conf.GetAutomatic() && !ownerAction && (newCheckpoint || retryLimit) && !now.Before(due)

		// Only the actor runs a pass; concurrent commands join its requested sequence.
		if snapshot != nil && (manual || automatic) {
			if !manual {
				requested = b.request(false)
			}
			result, err := a.runBackupPass(ctx, b, requested, conf, snapshot, engine, source, local)
			if errors.Is(err, block.ErrRootMoved) {
				select {
				case <-ctx.Done():
					return context.Cause(ctx)
				case <-checkpointChanged:
					continue
				}
			}
			if err != nil {
				return err
			}
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			if err := a.saveBackupResult(ctx, conf.GetId(), result); err != nil {
				return err
			}
			b.publish(result, true)
			if !conf.GetAutomatic() {
				return nil
			}
			attempted, lastPass = snapshot, time.Now()
			ownerAction = result.GetWaitingQuota() != nil || result.GetWaitingSignIn() != nil
			retryAt = time.Time{}
			if wait := result.GetWaitingUploadLimit(); wait != nil {
				retryAt = time.Unix(wait.GetResumeUnixSeconds(), 0)
			}
			continue
		}

		// Timers represent only a known minimum interval or upload retry deadline.
		var timer *time.Timer
		var deadline <-chan time.Time
		if conf.GetAutomatic() && !ownerAction && (newCheckpoint || !retryAt.IsZero()) && due.After(now) {
			timer = time.NewTimer(due.Sub(now))
			deadline = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return context.Cause(ctx)
		case <-checkpointChanged:
		case <-changed:
		case <-deadline:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

// saveBackupResult records a terminal outcome before a watcher can observe it.
// A process restart therefore keeps quota/sign-in waits and upload deadlines.
func (a *ProviderAccount) saveBackupResult(ctx context.Context, id string, state *account_settings.SpaceBackupState) error {
	// Encode public metadata without a transform, credential or remote operation.
	ctx = block.WithBackground(ctx)
	data, err := state.MarshalVT()
	if err != nil {
		return err
	}
	store, err := a.spaceBackupRegistryStore()
	if err != nil {
		return err
	}
	open := func(ctx context.Context) (kvtx.Tx, error) { return store.NewTransaction(ctx, true) }
	return kvtx.RunTransaction(ctx, true, open, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, []byte("state/"+id), data)
	})
}

// runBackupPass pins locally, copies ciphertext, and moves the hold only after publication.
func (a *ProviderAccount) runBackupPass(
	ctx context.Context, b *SpaceBackup, sequence uint64, conf *account_settings.SpaceBackup,
	snapshot *account_settings.SpaceBackupSnapshot, engine *space_world.Engine,
	source block.StoreOps, local kvtx.Store,
) (*account_settings.SpaceBackupState, error) {
	// A stale capture retries from the next checkpoint; no index can name an unpinned root.
	state := &account_settings.SpaceBackupState{RunSequence: sequence}
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return backupFailure(state, err), nil
	}
	backend := settings.FindStorageBackend(conf.GetStorageBackendId())
	if backend.GetGdrive() == nil {
		return backupFailure(state, errors.New("Drive backend no longer exists; configure a new backup")), nil
	}

	// Use only the account's public entity keypairs, as SObject recovery does.
	var recipients []crypto.PubKey
	for _, pair := range settings.GetEntityKeypairs() {
		pub, err := session.ExtractPublicKeyFromPeerID(pair.GetPeerId())
		if err != nil {
			return backupFailure(state, errors.New("account recovery public key is invalid")), nil
		}
		recipients = append(recipients, pub)
	}
	roots := packcopy_world.RootTypes{}
	for _, id := range objecttypes.BuiltInObjectTypeIDs() {
		value, _, reference, err := bus.ExecOneOffTyped[blocktype.LookupBlockTypeValue](ctx, a.t.p.b,
			blocktype.NewLookupBlockType(id), bus.ReturnWhenIdle(), nil)
		if reference != nil {
			defer reference.Release()
		}
		if err == nil && value != nil {
			roots[id] = value.GetValue().Constructor
		}
	}

	// Open the destination only after Pass's unchanged-root check.
	prefix := conf.GetSpaceId() + "/"
	destination := backend.GetId() + "/" + backend.GetGdrive().GetRootFolderId() + "/" + prefix
	pass := &space_backup.Pass{
		Log: a.le, Snapshot: snapshot, Source: source, Local: local,
		Destination: destination, Prefix: prefix, RootTypes: roots,
		Pin:               func(ctx context.Context) error { return engine.PinBackup(ctx, conf.GetId(), snapshot, local) },
		ProviderAccountID: a.GetAccountID(), EntityID: a.GetAccountID(), Recipients: recipients,
		Open: func(ctx context.Context, index kvtx.Store) (*block_store_s3.PackStore, block_store_s3.ObjectStore, func(), error) {
			client, err := a.openGdriveClient(ctx, backend)
			if err != nil {
				return nil, nil, nil, err
			}
			store := block_store_s3.NewPackStore(a.le, client, "", prefix, &block_store_s3.PackStoreOptions{
				KeepPacks: true, SoleWriter: true, Index: index,
			})
			return store, client, store.Close, nil
		},
	}
	published, counts, err := pass.Run(ctx, func(counts *account_settings.SpaceBackupProgress) {
		b.publish(&account_settings.SpaceBackupState{RunSequence: sequence, State: &account_settings.SpaceBackupState_Copying{Copying: counts}}, false)
	})
	if errors.Is(err, block.ErrRootMoved) {
		return nil, err
	}
	if err != nil {
		return backupFailure(state, err), nil
	}
	if published == nil {
		state.State = &account_settings.SpaceBackupState_Incomplete{Incomplete: counts}
		return state, nil
	}

	// Keep the published snapshot on this device without changing a Space object.
	if err := engine.CompleteBackup(ctx, conf.GetId(), snapshot, local); err != nil {
		return backupFailure(state, err), nil
	}
	state.State = &account_settings.SpaceBackupState_UpToDate{UpToDate: &account_settings.SpaceBackupPublication{
		Key: published.GetKey(), Seqno: published.GetSnapshot().GetWorld().GetSeqno(), PublishedUnixSeconds: published.GetPublishedUnixSeconds(),
	}}
	return state, nil
}

// backupFailure maps recoverable Drive outcomes into states with a next action.
func backupFailure(state *account_settings.SpaceBackupState, err error) *account_settings.SpaceBackupState {
	// Google supplies no exact reset timestamp, so wait a conservative full day.
	switch {
	case errors.Is(err, block_store_gdrive.ErrUploadLimit):
		state.State = &account_settings.SpaceBackupState_WaitingUploadLimit{
			WaitingUploadLimit: &account_settings.SpaceBackupWaiting{ResumeUnixSeconds: time.Now().Add(24 * time.Hour).Unix()},
		}
	case errors.Is(err, block_store_gdrive.ErrQuotaFull):
		state.State = &account_settings.SpaceBackupState_WaitingQuota{WaitingQuota: &account_settings.SpaceBackupWaiting{}}
	case errors.Is(err, block_store_gdrive.ErrSignIn):
		state.State = &account_settings.SpaceBackupState_WaitingSignIn{WaitingSignIn: &account_settings.SpaceBackupWaiting{}}
	default:
		state.State = &account_settings.SpaceBackupState_Failed{
			Failed: &account_settings.SpaceBackupFailure{Message: err.Error() + "; retry the backup to resume"},
		}
	}
	return state
}
