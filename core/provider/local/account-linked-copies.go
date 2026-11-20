package provider_local

import (
	"context"
	"os"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/peer"
)

// LinkedFullCopyEnv names the environment variable that, when set to "on",
// makes every account replica copy on this Device keep a full copy, as
// SetKeepLinkedCopies(true) does: the copy fetches the data of every block,
// including the chunks only a linked file on another Device holds, as account
// replication did before linked files. It is read once, at startup.
const LinkedFullCopyEnv = "SPACEWAVE_LINKED_FULL_COPY"

// linkedFullCopy selects LinkedFullCopyEnv's whole copy.
var linkedFullCopy = os.Getenv(LinkedFullCopyEnv) == "on"

// linkedCopiesKey is the account object store key of the Device's
// keep-full-copy setting. A missing key reads as off.
var linkedCopiesKey = []byte("linked-copies/keep-full")

// KeepLinkedCopies reports whether this Device keeps a full copy of its
// account's Spaces, including the chunks only a linked file on another Device
// of the account holds. Off by default: account replication then copies a
// linked Drive's tree and Blobs and leaves its file chunks on the linking
// Device, which serves them.
func (a *ProviderAccount) KeepLinkedCopies(ctx context.Context) (bool, error) {
	// Open the account object store.
	objStore, release, err := a.buildSoObjectStore(ctx)
	if err != nil {
		return false, err
	}
	defer release()

	// Read the setting; a missing key reads as off.
	var keep bool
	openRead := func(ctx context.Context) (kvtx.Tx, error) { return objStore.NewTransaction(ctx, false) }
	err = kvtx.RunTransaction(ctx, false, openRead, func(ctx context.Context, tx kvtx.Tx) error {
		data, found, err := tx.Get(ctx, linkedCopiesKey)
		keep = found && len(data) == 1 && data[0] == 1
		return err
	})
	return keep, err
}

// SetKeepLinkedCopies stores this Device's keep-full-copy setting, then wakes
// each account replica copy, which copies its newest head again under the new
// setting: turned on, the copy rechecks every block and fetches the linked
// chunks it skipped.
func (a *ProviderAccount) SetKeepLinkedCopies(ctx context.Context, keep bool) error {
	// Open the account object store.
	objStore, release, err := a.buildSoObjectStore(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Store the setting, then wake the replica copies.
	var data byte
	if keep {
		data = 1
	}
	openWrite := func(ctx context.Context) (kvtx.Tx, error) { return objStore.NewTransaction(ctx, true) }
	err = kvtx.RunTransaction(ctx, true, openWrite, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, linkedCopiesKey, []byte{data})
	})
	if err != nil {
		return err
	}
	a.linkedCopiesGen.SwapValue(func(gen uint64) uint64 { return gen + 1 })
	return nil
}

// linkedCopy is how one account replica walk treats the chunks a linked file
// serves.
type linkedCopy struct {
	// full fetches the data of every block, as before linked files, and counts
	// no linked block as present.
	full bool
	// recheck trusts no completion proof: an earlier walk counted linked
	// chunks as present, so its proofs may cover subtrees missing them.
	recheck bool
	// peers admits the peers whose linked answers the walk honours, the
	// unrevoked sessions of this account when the walk began; nil when it
	// honours none.
	peers func(peer.ID) bool
}

// readLinkedCopy reads, once per walk, how the walk treats linked chunks.
// linkedProofs is the progress record's LinkedProofs. The account's sessions
// are a snapshot: a session revoked during the walk is honoured until the
// next walk.
func (a *ProviderAccount) readLinkedCopy(ctx context.Context, linkedProofs bool) (linkedCopy, error) {
	// Keep a full copy when this Device's setting or the environment says so.
	full := linkedFullCopy
	if !full {
		keep, err := a.KeepLinkedCopies(ctx)
		if err != nil {
			return linkedCopy{}, err
		}
		full = keep
	}
	if full {
		return linkedCopy{full: true, recheck: linkedProofs}, nil
	}

	// Honour linked answers only from this account's own sessions: a member
	// answering linked falsely would make this copy skip a block the member
	// could then withhold. An account without settings has no other session.
	settings, err := a.readAccountSettings(ctx)
	if errors.Is(err, sobject.ErrSharedObjectNotFound) {
		return linkedCopy{}, nil
	}
	if err != nil {
		return linkedCopy{}, err
	}
	return linkedCopy{peers: accountSessionPeers(settings)}, nil
}

// accountSessionPeers admits the unrevoked sessions of settings.
func accountSessionPeers(settings *account_settings.AccountSettings) func(peer.ID) bool {
	return func(id peer.ID) bool {
		session := settings.FindAccountSession(id.String())
		return session != nil && !session.GetRevoked()
	}
}
