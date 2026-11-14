package provider_local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// ReplaceOrigin makes ReplaceSharedObjectState a compare-and-set that
// records the writer's origin.
type ReplaceOrigin struct {
	// Generation is the list entry generation the object must have.
	Generation string
	// State is the stored host state the write expects, nil for none.
	State []byte
	// Source is stored as the origin's source.
	Source []byte
}

// ReplaceSharedObjectState replaces the stored state and config history of
// the listed shared object id with a genesis owned by owner that carries
// stateData, then restarts its tracker so every mount reads the new state. It
// checks the listing in the write transaction: an unlisted id returns
// ErrSharedObjectNotFound and writes nothing, so a write that races a delete
// leaves no state that no delete can reach. The write also requires the
// listed generation and the stored state to equal the ones origin names, else
// returns ErrStateChanged and writes nothing, and stores a written
// SharedObjectOrigin with the new state's hash in the same transaction.
//
// Invariants:
//   - No writer mounted on the target commits over or under a replace. Write
//     transactions on the account store hold its writer from open to end, so
//     each mounted write commits wholly before or wholly after the replace.
//     A mounted state write commits only over the stored bytes its mount
//     loaded or last wrote (NewObjectStoreSOStateFuncs), so one after the
//     replace finds the new state and fails with ErrStateChanged, from the
//     commit until the restart below ends the mount, and the restarted
//     mount loads the new state.
//   - A replace never commits over a state newer than the one its caller
//     read. A mounted write acknowledged before the replace changed the
//     stored bytes, so the replace returns ErrStateChanged and writes
//     nothing; the caller reads again and decides.
//   - Every mount reads the stored state after the replace returns, even
//     when the commit returned an error after storing it: any failure but a
//     refusal restarts the mount too.
//   - Keys a mount writes outside the host state, its local state stores
//     and join requests, are not part of the replace and are kept.
func (a *ProviderAccount) ReplaceSharedObjectState(ctx context.Context, le *logrus.Entry, id string, owner crypto.PrivKey, stateData []byte, origin ReplaceOrigin) error {
	// Build the genesis before the transaction.
	state, genesis, err := sobject.BuildGenesisSOState(le, a.t.p.sfs, id, owner, stateData)
	if err != nil {
		return err
	}
	data, err := state.MarshalVT()
	if err != nil {
		return err
	}
	stateHash := sha256.Sum256(data)

	// Replace the state of the listed object.
	err = a.runSoObjectStoreTx(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		// Write only an object the account lists.
		listed, err := storedListsSharedObject(ctx, tx, id)
		if err != nil {
			return err
		}
		if !listed {
			return sobject.ErrSharedObjectNotFound
		}

		// Write only the generation and state the caller read, and record
		// the write as the object's origin.
		entry, _, err := readListedOrigin(ctx, tx, id)
		if err != nil {
			return err
		}
		stored, found, err := tx.Get(ctx, SobjectObjectStoreHostStateKey(id))
		if err != nil {
			return err
		}
		if entry.GetGeneration() != origin.Generation || found != (origin.State != nil) || !bytes.Equal(stored, origin.State) {
			return ErrStateChanged
		}
		err = writeSharedObjectOrigin(ctx, tx, id, &SharedObjectOrigin{
			Generation: origin.Generation,
			StateHash:  stateHash[:],
			Source:     origin.Source,
		})
		if err != nil {
			return err
		}

		// The genesis starts a new lineage, so drop the old history checkpoint.
		if err := tx.Delete(ctx, SOConfigHistoryCheckpointKey(id)); err != nil {
			return err
		}
		if err := WriteSOConfigHistory(ctx, tx, id, genesis.GetConfig(), state.GetConfig(), []*sobject.SOConfigChange{genesis}); err != nil {
			return err
		}
		return tx.Set(ctx, SobjectObjectStoreHostStateKey(id), data)
	})
	if errors.Is(err, ErrStateChanged) || errors.Is(err, sobject.ErrSharedObjectNotFound) {
		return err
	}
	if err == nil && testHookReplaceCommitted != nil {
		err = testHookReplaceCommitted(id)
	}

	// Restart the mount whether the commit succeeded or failed: a commit
	// error can follow a stored write, and the restarted mount reads
	// whatever storage holds.
	a.restartSharedObject(id, nil)
	return err
}
