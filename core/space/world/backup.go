package space_world

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// backupStateStoreID keeps root holds, copy proofs and publication records on
// this device. It is never an object or retained-root operation in the Space.
const backupStateStoreID = "world-backups"

// BackupSnapshot returns the installed checkpoint and head's retained roots,
// together with the next change notification. It performs no block I/O.
func (e *Engine) BackupSnapshot() (*account_settings.SpaceBackupSnapshot, <-chan struct{}) {
	// Capture the root set and its notification under the adapter's single guard.
	var snapshot *account_settings.SpaceBackupSnapshot
	var changed <-chan struct{}
	e.a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		changed = getWaitCh()
		if !e.a.installed {
			return
		}
		snapshot = &account_settings.SpaceBackupSnapshot{World: &s4wave_world.WorldRootSnapshot{
			RootRef: e.a.checkpoint.GetHeadRef().CloneVT(), Seqno: e.a.checkpointSeqno,
		}}
		for _, root := range e.a.headState.GetRetainedRoots() {
			snapshot.RetainedRoots = append(snapshot.RetainedRoots, root.GetRootRef().Clone())
		}
	})
	return snapshot, changed
}

// AccessBackupStateStore opens the device-local store, whose caller releases
// it after its pass or watch. Its keys never enter the replicated Space.
func (e *Engine) AccessBackupStateStore(ctx context.Context) (kvtx.Store, func(), error) {
	return e.a.so.AccessLocalStateStore(ctx, backupStateStoreID, nil)
}

// PinBackup holds a still-current root set before its copy starts. It writes
// only local named roots and local metadata; it never queues a Space operation.
//
// Invariant: the old root remains held while a replacement hold record commits.
// A crash between the append and compaction keeps extra roots, never fewer.
// No collector setting, deletion policy or remote storage placement changes.
func (e *Engine) PinBackup(ctx context.Context, id string, snapshot *account_settings.SpaceBackupSnapshot, local kvtx.Store) error {
	// Flush kept pages outside every metadata and World root lock.
	ctx = block.WithBackground(ctx)
	roots := backupRoots(snapshot)
	if id == "" || snapshot.GetWorld().GetRootRef().GetRootRef().GetEmpty() {
		return errors.New("backup ID and checkpoint are required")
	}
	endFlush, err := e.a.pages.Flush(ctx, roots)
	if err != nil {
		return err
	}
	defer endFlush()

	// Serialize the hold manifest independently from every foreground writer.
	release, err := e.a.backupMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Append before changing the manifest, with no sync wait under the root lock.
	if err := e.appendBackupHold(ctx, snapshot, roots); err != nil {
		return err
	}

	// Commit the new running pin, then compact to running and newest published roots.
	if err := writeBackupHold(ctx, local, "pins/"+id, snapshot, false); err != nil {
		return err
	}
	if _, err := e.a.pages.Sync(ctx); err != nil {
		return err
	}
	return e.compactBackupHold(ctx, local)
}

// appendBackupHold atomically takes over the still-held checkpoint root.
// Its ordered swap cannot wait for a disk durability fence under rootMtx.
func (e *Engine) appendBackupHold(ctx context.Context, snapshot *account_settings.SpaceBackupSnapshot, roots []*block.BlockRef) error {
	// Revalidate the source hold in the same guard that serializes checkpoint moves.
	release, err := e.a.rootMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	current, _ := e.BackupSnapshot()
	if !current.EqualVT(snapshot) {
		return block.ErrRootMoved
	}
	if !refEqual(e.a.held[RootReplayBase], snapshot.GetWorld().GetRootRef().GetRootRef()) {
		return block.ErrRootMoved
	}

	// Keep the old backup chain until its replacement manifest is durable.
	previous := e.a.held[RootBackup]
	refs := slices.Clone(roots)
	if !previous.GetEmpty() {
		refs = append(refs, previous)
	}
	entry, err := refsEntry(&HeldRoots{Roots: roots, Prev: previous}, refs)
	if err != nil {
		return err
	}
	return e.a.moveRootsLocked(kvtx.WithOrderedCommitContext(ctx), []*block.PutBatchEntry{entry},
		block.NamedRoot{Name: RootBackup, Ref: entry.Ref, RequireHeld: true})
}

// CompleteBackup replaces this backup's local running hold with its newest
// published index's roots. It is called only after the index and local proof
// of publication are durable. It never releases another backup's roots.
func (e *Engine) CompleteBackup(ctx context.Context, id string, snapshot *account_settings.SpaceBackupSnapshot, local kvtx.Store) error {
	// Serialize only backup manifests; foreground writers never take this lock.
	ctx = block.WithBackground(ctx)
	release, err := e.a.backupMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := writeBackupHold(ctx, local, "published/"+id, snapshot, true); err != nil {
		return err
	}
	return e.compactBackupHold(ctx, local)
}

// backupRoots returns only the roots the hold must retain.
func backupRoots(snapshot *account_settings.SpaceBackupSnapshot) []*block.BlockRef {
	return append([]*block.BlockRef{snapshot.GetWorld().GetRootRef().GetRootRef()}, snapshot.GetRetainedRoots()...)
}

// writeBackupHold persists one device-local snapshot and, on publication,
// drops only the matching running metadata row in that same transaction.
func writeBackupHold(ctx context.Context, local kvtx.Store, key string, snapshot *account_settings.SpaceBackupSnapshot, published bool) error {
	// Marshal before opening the short local write transaction.
	data, err := snapshot.MarshalVT()
	if err != nil {
		return err
	}
	open := func(ctx context.Context) (kvtx.Tx, error) { return local.NewTransaction(ctx, true) }
	return kvtx.RunTransaction(ctx, true, open, func(ctx context.Context, tx kvtx.Tx) error {
		if err := tx.Set(ctx, []byte(key), data); err != nil {
			return err
		}
		if published {
			return tx.Delete(ctx, []byte("pins/"+key[len("published/"):]))
		}
		return nil
	})
}

// compactBackupHold keeps only what local hold rows name. The caller holds
// backupMtx; its read and encoding finish before taking the World root lock.
func (e *Engine) compactBackupHold(ctx context.Context, local kvtx.Store) error {
	// Stream the bounded metadata rows, retaining only the resulting root set.
	roots, err := readBackupHolds(ctx, local)
	if err != nil {
		return err
	}
	entry, err := refsEntry(&HeldRoots{Roots: roots}, roots)
	if err != nil {
		return err
	}

	// A crash before this ordered compaction reaches disk keeps the old, larger hold.
	release, err := e.a.rootMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	return e.a.moveRootsLocked(kvtx.WithOrderedCommitContext(ctx), []*block.PutBatchEntry{entry},
		block.NamedRoot{Name: RootBackup, Ref: entry.Ref, RequireHeld: true})
}

// readBackupHolds streams each local running and published hold once.
func readBackupHolds(ctx context.Context, local kvtx.Store) ([]*block.BlockRef, error) {
	// Release the read transaction before its caller moves the named root.
	tx, err := local.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	var roots []*block.BlockRef
	for _, prefix := range []string{"pins/", "published/"} {
		if err := tx.ScanPrefix(ctx, []byte(prefix), func(_, data []byte) error {
			snapshot := &account_settings.SpaceBackupSnapshot{}
			if err := snapshot.UnmarshalVT(data); err != nil {
				return err
			}
			roots = append(roots, backupRoots(snapshot)...)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return dedupRefs(roots), nil
}
