package provider_spacewave

import (
	"bytes"
	"context"
	"encoding/hex"
	"sync"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_statestore "github.com/s4wave/spacewave/core/sobject/statestore"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
)

// verifiedSOStateCacheKeyPrefix is the ObjectStore key prefix for verified SO cache state.
const verifiedSOStateCacheKeyPrefix = "verified-so-state/"

// verifiedSOHistoryKeyPrefix is the ObjectStore key prefix for retained
// verified config changes. Each change is stored once under its hash.
const verifiedSOHistoryKeyPrefix = "verified-so-history/"

// verifiedSOLogKeyPrefix is the ObjectStore key prefix of the verified SO
// cache's state logs, outside verifiedSOStateCacheKeyPrefix so a record's key
// is never a prefix of another object's logs.
const verifiedSOLogKeyPrefix = "verified-so-log/"

// verifiedStateLog selects storing the cache's PeerState and CloudState as
// state logs (sobject_statestore.LayoutEnv) over inside the record. It is
// read once, here, at startup.
//
// The record holds the states only in the whole layout, and every write in
// either layout leaves the other layout's states empty: a log write stores
// the record without them, and a whole write after a mount's first clears
// the logs. A load reads the states from the record, or from the logs when
// the record holds none, so switching layouts loses no accepted state.
var verifiedStateLog = sobject_statestore.LogSelected()

// verifiedSOStateLogs returns the state logs of soID's PeerState and
// CloudState.
func verifiedSOStateLogs(soID string) (peer, cloud sobject_statestore.Store) {
	prefix := verifiedSOLogKeyPrefix + soID + "/"
	return sobject_statestore.New([]byte(prefix + "peer/")), sobject_statestore.New([]byte(prefix + "cloud/"))
}

// verifiedSOStateCacheKey returns the ObjectStore key for a verified SO cache entry.
func verifiedSOStateCacheKey(soID string) []byte {
	return []byte(verifiedSOStateCacheKeyPrefix + soID)
}

// verifiedSOHistoryPrefix returns the ObjectStore key prefix of soID's retained history.
func verifiedSOHistoryPrefix(soID string) []byte {
	return []byte(verifiedSOHistoryKeyPrefix + soID + "/")
}

// verifiedSOHistoryKey returns the ObjectStore key of one retained config change.
func verifiedSOHistoryKey(soID string, hash []byte) []byte {
	return append(verifiedSOHistoryPrefix(soID), hex.EncodeToString(hash)...)
}

// writeVerifiedSOStateCache serializes verified SO cache state to the account
// ObjectStore and retains changes in the same transaction. Changes already
// retained are not written again. It knows no earlier write, so in the log
// layout it replaces both state logs; a mount writes through
// verifiedSOCacheWriter instead, which stores each write's delta.
func (a *ProviderAccount) writeVerifiedSOStateCache(
	ctx context.Context,
	soID string,
	cache *api.VerifiedSOStateCache,
	changes []*sobject.SOConfigChange,
) error {
	// Commit the cache before invalidating state derived from it.
	_, err := writeVerifiedSOStateCacheFrom(ctx, a.objStore, soID, cache, changes, nil)
	if err == nil {
		a.invalidateSelfEnrollmentEntry(soID)
	}
	return err
}

// verifiedSOWritten holds what a committed write stored in the cache's
// state logs: each state and its head, nil when the log is empty.
type verifiedSOWritten struct {
	peer, cloud verifiedSOLogged
}

// verifiedSOLogged is one state log as a write left it.
type verifiedSOLogged struct {
	state *sobject.SOState
	head  []byte
}

// writeVerifiedSOStateCacheFrom writes cache over prev, what the last
// committed write stored in the logs, and returns what this write stored.
// A nil prev means the logs are unknown: a log write replaces them and a
// whole write clears them. A failed write returns nil, since a commit error
// can follow a stored write. It takes the store and leaves invalidation to
// the caller.
func writeVerifiedSOStateCacheFrom(
	ctx context.Context,
	store object.ObjectStore,
	soID string,
	cache *api.VerifiedSOStateCache,
	changes []*sobject.SOConfigChange,
	prev *verifiedSOWritten,
) (*verifiedSOWritten, error) {
	// Validate the store and arguments.
	if store == nil {
		return nil, errors.New("account object store not ready")
	}
	if soID == "" {
		return nil, errors.New("shared object id is required")
	}
	if cache == nil {
		return nil, errors.New("verified SO cache is required")
	}

	// Encode the record, without the states in the log layout.
	record := cache
	if verifiedStateLog {
		rest := *cache
		rest.PeerState, rest.CloudState = nil, nil
		record = &rest
	}
	data, err := record.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal verified SO cache")
	}

	// Write the record, the logs and the new changes in one transaction.
	var written *verifiedSOWritten
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Write the verified state record.
			if err := tx.Set(ctx, verifiedSOStateCacheKey(soID), data); err != nil {
				return errors.Wrap(err, "set verified SO cache")
			}

			// Write the states to the logs, or clear the logs once.
			var err error
			written, err = writeVerifiedSOStateLogs(ctx, tx, soID, cache, prev)
			if err != nil {
				return errors.Wrap(err, "write verified SO state logs")
			}

			// Append each change not yet retained under its hash.
			for _, change := range changes {
				if err := writeVerifiedSOHistoryEntry(ctx, tx, soID, change); err != nil {
					return err
				}
			}
			return nil
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "write verified SO cache")
	}
	return written, nil
}

// writeVerifiedSOStateLogs stores the cache's states in their logs over prev
// in the log layout, and clears the logs in the whole layout unless prev
// shows them empty.
func writeVerifiedSOStateLogs(ctx context.Context, tx kvtx.TxOps, soID string, cache *api.VerifiedSOStateCache, prev *verifiedSOWritten) (*verifiedSOWritten, error) {
	// The whole layout leaves the logs empty.
	peerLog, cloudLog := verifiedSOStateLogs(soID)
	if !verifiedStateLog {
		if prev == nil || prev.peer.state != nil || prev.cloud.state != nil {
			if err := peerLog.Clear(ctx, tx); err != nil {
				return nil, err
			}
			if err := cloudLog.Clear(ctx, tx); err != nil {
				return nil, err
			}
		}
		return &verifiedSOWritten{}, nil
	}

	// The log layout stores each state by its delta.
	var known verifiedSOWritten
	if prev != nil {
		known = *prev
	}
	peer, err := writeVerifiedSOStateLog(ctx, tx, peerLog, known.peer, prev != nil, cache.GetPeerState())
	if err != nil {
		return nil, err
	}
	cloud, err := writeVerifiedSOStateLog(ctx, tx, cloudLog, known.cloud, prev != nil, cache.GetCloudState())
	if err != nil {
		return nil, err
	}
	return &verifiedSOWritten{peer: peer, cloud: cloud}, nil
}

// writeVerifiedSOStateLog stores next in log over prev, which known reports
// is what the log holds, and returns what it stored. A nil next empties the
// log.
func writeVerifiedSOStateLog(
	ctx context.Context,
	tx kvtx.TxOps,
	log sobject_statestore.Store,
	prev verifiedSOLogged,
	known bool,
	next *sobject.SOState,
) (verifiedSOLogged, error) {
	// Empty the log for no state, unless it is known empty.
	if next == nil {
		if known && prev.state == nil {
			return prev, nil
		}
		return verifiedSOLogged{}, log.Clear(ctx, tx)
	}

	// Store the delta over a known state the log still holds under its head,
	// else the whole state: another write, such as InvalidateVerifiedChain,
	// may have changed the log since prev.
	stored := known && prev.state != nil
	if stored {
		head, found, err := tx.Get(ctx, log.HeadKey())
		if err != nil {
			return verifiedSOLogged{}, err
		}
		stored = found && bytes.Equal(head, prev.head)
	}
	var head []byte
	var err error
	if stored {
		head, err = log.Write(ctx, tx, prev.head, prev.state, next)
	} else {
		head, err = log.Replace(ctx, tx, next)
	}
	if err != nil {
		return verifiedSOLogged{}, err
	}
	return verifiedSOLogged{state: next, head: head}, nil
}

// verifiedSOCacheWriter persists one mount's verified SO cache. It remembers
// what its last committed write stored in the logs, so each write stores
// only its delta; after a failed write it forgets, and the next write
// replaces the logs.
type verifiedSOCacheWriter struct {
	// store is the account's ObjectStore.
	store object.ObjectStore
	// soID identifies the SharedObject whose verified cache is written.
	soID string
	// onWritten runs after each committed write with the SharedObject ID,
	// to drop state derived from the cache.
	onWritten func(soID string)
	// mtx serializes the writes, so written is the store's state.
	mtx sync.Mutex
	// written is what the last committed write stored, nil when unknown.
	written *verifiedSOWritten
}

// write persists cache and retains changes.
func (w *verifiedSOCacheWriter) write(ctx context.Context, cache *api.VerifiedSOStateCache, changes []*sobject.SOConfigChange) error {
	// Serialize writes over the last committed cache state.
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Commit the cache and invalidate derived state only on success.
	written, err := writeVerifiedSOStateCacheFrom(ctx, w.store, w.soID, cache, changes, w.written)
	if err == nil && w.onWritten != nil {
		w.onWritten(w.soID)
	}

	// Remember the committed state, or forget it after a failed write.
	w.written = written
	return err
}

// writeVerifiedSOHistoryEntry stores change under its hash unless it is already retained.
func writeVerifiedSOHistoryEntry(ctx context.Context, tx kvtx.Tx, soID string, change *sobject.SOConfigChange) error {
	// Address the change by its hash and skip it when already retained.
	hash, err := sobject.HashSOConfigChange(change)
	if err != nil {
		return errors.Wrap(err, "hash verified SO history entry")
	}
	key := verifiedSOHistoryKey(soID, hash)
	found, err := tx.Exists(ctx, key)
	if err != nil || found {
		return err
	}

	// Store the serialized change.
	data, err := change.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal verified SO history entry")
	}
	if err := tx.Set(ctx, key, data); err != nil {
		return errors.Wrap(err, "set verified SO history entry")
	}
	return nil
}

// readVerifiedSOHistoryEntry returns the retained config change of soID with
// hash, or nil when it is not retained.
func (a *ProviderAccount) readVerifiedSOHistoryEntry(
	ctx context.Context,
	soID string,
	hash []byte,
) (*sobject.SOConfigChange, error) {
	// Validate the store.
	if a.objStore == nil {
		return nil, errors.New("account object store not ready")
	}

	// Read and decode the entry, leaving it nil when absent.
	var entry *sobject.SOConfigChange
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return a.objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Reset the result so a retried attempt starts clean.
			entry = nil
			data, found, err := tx.Get(ctx, verifiedSOHistoryKey(soID, hash))
			if err != nil || !found {
				return err
			}
			entry = &sobject.SOConfigChange{}
			return entry.UnmarshalVT(data)
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "read verified SO history entry")
	}
	return entry, nil
}

// deleteVerifiedSOStateCache removes the persisted verified SO cache entry,
// its state logs and the retained history for soID, if any. Missing entries are not an
// error: the caller is asking for a cold next mount, which a missing entry
// already produces.
func (a *ProviderAccount) deleteVerifiedSOStateCache(
	ctx context.Context,
	soID string,
) error {
	// Validate the store and arguments.
	if a.objStore == nil {
		return errors.New("account object store not ready")
	}
	if soID == "" {
		return errors.New("shared object id is required")
	}

	// Delete the state record and its history in one transaction.
	err := kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return a.objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Delete the verified state record.
			if err := tx.Delete(ctx, verifiedSOStateCacheKey(soID)); err != nil {
				return errors.Wrap(err, "delete verified SO cache")
			}

			// Clear both state logs.
			peerLog, cloudLog := verifiedSOStateLogs(soID)
			if err := peerLog.Clear(ctx, tx); err != nil {
				return errors.Wrap(err, "clear verified SO peer state")
			}
			if err := cloudLog.Clear(ctx, tx); err != nil {
				return errors.Wrap(err, "clear verified SO cloud state")
			}

			// Collect the retained history keys, then delete them.
			var keys [][]byte
			err := tx.ScanPrefixKeys(ctx, verifiedSOHistoryPrefix(soID), func(key []byte) error {
				keys = append(keys, bytes.Clone(key))
				return nil
			})
			if err != nil {
				return errors.Wrap(err, "list verified SO history")
			}
			for _, key := range keys {
				if err := tx.Delete(ctx, key); err != nil {
					return errors.Wrap(err, "delete verified SO history")
				}
			}
			return nil
		},
	)
	if err != nil {
		return errors.Wrap(err, "delete verified SO cache")
	}
	a.invalidateSelfEnrollmentEntry(soID)
	return nil
}

// InvalidateVerifiedChain clears the persisted verified config-chain record
// for soID so the next mount re-verifies from scratch via /config-chain.
// Used by rejoin, recovery, and forced re-verification flows where the
// previously trusted chain head is no longer authoritative. The live
// cloudSOHost (if any) keeps its in-memory verified state for the
// remainder of its session; only future mounts observe the cold cache.
func (a *ProviderAccount) InvalidateVerifiedChain(ctx context.Context, soID string) error {
	return a.deleteVerifiedSOStateCache(ctx, soID)
}

// loadVerifiedSOStateCache reads verified SO cache state from the account
// ObjectStore, its states from the record or from their logs.
func (a *ProviderAccount) loadVerifiedSOStateCache(
	ctx context.Context,
	soID string,
) (*api.VerifiedSOStateCache, error) {
	cache, _, err := a.loadVerifiedSOStateCacheFrom(ctx, soID)
	return cache, err
}

// loadVerifiedSOStateCacheFrom reads the cache like loadVerifiedSOStateCache
// and returns what the logs hold for a writer to start from, nil when a first
// write must replace or clear them: the record held a state, or no record
// was found.
func (a *ProviderAccount) loadVerifiedSOStateCacheFrom(
	ctx context.Context,
	soID string,
) (*api.VerifiedSOStateCache, *verifiedSOWritten, error) {
	// Validate the store and arguments.
	if a.objStore == nil {
		return nil, nil, errors.New("account object store not ready")
	}
	if soID == "" {
		return nil, nil, errors.New("shared object id is required")
	}

	// Read and decode the record, leaving it nil when absent.
	var cache *api.VerifiedSOStateCache
	var written *verifiedSOWritten
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return a.objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Reset the result so a retried attempt starts clean.
			cache, written = nil, nil
			data, found, err := tx.Get(ctx, verifiedSOStateCacheKey(soID))
			if err != nil {
				return errors.Wrap(err, "get verified SO cache")
			}
			if !found {
				return nil
			}

			// Decode the record.
			next := &api.VerifiedSOStateCache{}
			if err := next.UnmarshalVT(data); err != nil {
				return errors.Wrap(err, "unmarshal verified SO cache")
			}
			cache = next

			// A record holding a state was written whole, so its logs are empty
			// or stale; the first write clears or replaces them.
			if cache.GetPeerState() != nil || cache.GetCloudState() != nil {
				return nil
			}

			// Otherwise the states are in their logs.
			peerLog, cloudLog := verifiedSOStateLogs(soID)
			peer, err := loadVerifiedSOStateLog(ctx, tx, peerLog)
			if err != nil {
				return errors.Wrap(err, "load verified SO peer state")
			}
			cloud, err := loadVerifiedSOStateLog(ctx, tx, cloudLog)
			if err != nil {
				return errors.Wrap(err, "load verified SO cloud state")
			}
			cache.PeerState, cache.CloudState = peer.state, cloud.state
			written = &verifiedSOWritten{peer: peer, cloud: cloud}
			return nil
		},
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "open read transaction")
	}
	return cache, written, nil
}

// loadVerifiedSOStateLog reads one state log, empty when it holds no state.
func loadVerifiedSOStateLog(ctx context.Context, tx kvtx.TxOps, log sobject_statestore.Store) (verifiedSOLogged, error) {
	state, head, found, err := log.Load(ctx, tx)
	if err != nil || !found {
		return verifiedSOLogged{}, err
	}
	return verifiedSOLogged{state: state, head: head}, nil
}
