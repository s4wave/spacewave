package provider_local

import (
	"bytes"
	"context"
	"encoding/hex"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
)

// SOConfigHistoryCheckpointKey addresses the locally held configuration from
// which retained history begins. Its value is a serialized SharedObjectConfig.
func SOConfigHistoryCheckpointKey(sharedObjectID string) []byte {
	return []byte("so/" + sharedObjectID + "/config-history/checkpoint")
}

// SOConfigHistoryEntryKey addresses a serialized SOConfigChange by its hash.
func SOConfigHistoryEntryKey(sharedObjectID string, hash []byte) []byte {
	return []byte("so/" + sharedObjectID + "/config-history/entry/" + hex.EncodeToString(hash))
}

// WriteSOConfigHistory retains verified transitions in the transaction that
// writes their resulting SOState. The caller commits both or discards both.
// Ordinary state writes do not establish or replace a history checkpoint.
// Retention records held trust; it does not authenticate legacy state or admit peers.
func WriteSOConfigHistory(
	ctx context.Context,
	tx kvtx.Tx,
	sharedObjectID string,
	current, next *sobject.SharedObjectConfig,
	changes []*sobject.SOConfigChange,
) error {
	// Ordinary writes leave retained history intact, including legacy history gaps.
	if len(changes) == 0 {
		return nil
	}
	if err := current.Validate(); err != nil {
		return errors.Wrap(err, "config history checkpoint")
	}

	// Verify every transition against the state held under the provider lock.
	checkpoint := current.CloneVT()
	for _, change := range changes {
		accepted, err := sobject.VerifyConfigChange(current, change)
		if err != nil {
			return err
		}
		if err := accepted.Validate(); err != nil {
			return errors.Wrap(err, "config history transition")
		}
		if len(checkpoint.GetConfigChainHash()) == 0 {
			checkpoint = accepted.CloneVT()
		}
		current = accepted
	}
	if !sobject.EqualSOConfigs(current, next) {
		return errors.New("config history does not match written state")
	}

	// Preserve the first locally held checkpoint across later writes and gaps.
	checkpointKey := SOConfigHistoryCheckpointKey(sharedObjectID)
	_, found, err := tx.Get(ctx, checkpointKey)
	if err != nil {
		return err
	}
	if !found {
		data, err := checkpoint.MarshalVT()
		if err != nil {
			return err
		}
		if err := tx.Set(ctx, checkpointKey, data); err != nil {
			return err
		}
	}

	// Store entries separately so opening state does not load the full history.
	for _, change := range changes {
		hash, err := sobject.HashSOConfigChange(change)
		if err != nil {
			return err
		}
		data, err := change.MarshalVT()
		if err != nil {
			return err
		}
		if err := tx.Set(ctx, SOConfigHistoryEntryKey(sharedObjectID, hash), data); err != nil {
			return err
		}
	}
	return nil
}

// NewSOHostSyncFuncs reads retained lineage from the object store.
// Local imports use the ordinary host lock, which commits state and lineage together.
func NewSOHostSyncFuncs(store kvtx.Store) *sobject.SOHostSyncFuncs {
	return &sobject.SOHostSyncFuncs{
		History: func(ctx context.Context, id string, base, target []byte) ([]*sobject.SOConfigChange, error) {
			// One read scope pins the complete bounded suffix across concurrent writes.
			tx, err := store.NewTransaction(ctx, false)
			if err != nil {
				return nil, err
			}
			defer tx.Discard()

			return readSOConfigHistory(ctx, tx, id, base, target)
		},
		Entry: func(ctx context.Context, id string, hash []byte) (*sobject.SOConfigChange, error) {
			tx, err := store.NewTransaction(ctx, false)
			if err != nil {
				return nil, err
			}
			defer tx.Discard()

			return readSOConfigEntry(ctx, tx, id, hash)
		},
	}
}

// WriteSOConfigCheckpoint records already-held trust in the caller's state transaction.
// Only an explicit authenticated invitation may replace an existing checkpoint.
func WriteSOConfigCheckpoint(ctx context.Context, tx kvtx.Tx, id string, config *sobject.SharedObjectConfig, replace bool) error {
	if len(config.GetConfigChainHash()) == 0 {
		return sobject.ErrConfigHistoryUnavailable
	}
	if err := config.Validate(); err != nil {
		return err
	}
	key := SOConfigHistoryCheckpointKey(id)
	if !replace {
		_, found, err := tx.Get(ctx, key)
		if err != nil || found {
			return err
		}
	}
	data, err := config.MarshalVT()
	if err != nil {
		return err
	}
	return tx.Set(ctx, key, data)
}

// readSOConfigHistory traverses immutable entries in the caller's read transaction.
func readSOConfigHistory(ctx context.Context, tx kvtx.Tx, id string, base, target []byte) ([]*sobject.SOConfigChange, error) {
	return sobject.ReadConfigSuffix(ctx, base, target, func(ctx context.Context, head []byte) (*sobject.SOConfigChange, error) {
		return readSOConfigEntry(ctx, tx, id, head)
	})
}

// readSOConfigEntry reads one retained entry, returning nil when it is absent.
func readSOConfigEntry(ctx context.Context, tx kvtx.Tx, id string, hash []byte) (*sobject.SOConfigChange, error) {
	data, found, err := tx.Get(ctx, SOConfigHistoryEntryKey(id, hash))
	if err != nil || !found {
		return nil, err
	}
	entry := &sobject.SOConfigChange{}
	if err := entry.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return entry, nil
}

// ReadSharedObjectConfigHistory returns accepted lineage from the local trust checkpoint.
func (s *SharedObject) ReadSharedObjectConfigHistory(ctx context.Context, target *sobject.SharedObjectConfig) (*sobject.SharedObjectConfig, []*sobject.SOConfigChange, error) {
	read, err := s.objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	defer read.Discard()
	data, found, err := read.Get(ctx, SOConfigHistoryCheckpointKey(s.GetSharedObjectID()))
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, sobject.ErrConfigHistoryUnavailable
	}
	base := &sobject.SharedObjectConfig{}
	if err := base.UnmarshalVT(data); err != nil {
		return nil, nil, err
	}
	changes, err := readSOConfigHistory(ctx, read, s.GetSharedObjectID(), base.GetConfigChainHash(), target.GetConfigChainHash())
	if err != nil {
		return nil, nil, err
	}
	if err := sobject.VerifyConfigChainSuffix(base, target, changes); err != nil {
		return nil, nil, err
	}
	return base, changes, nil
}

// ReadSharedObjectGenesis returns the signed entry behind the oldest retained
// checkpoint. Older replicas may require their original owner to supply it.
func (s *SharedObject) ReadSharedObjectGenesis(ctx context.Context, base *sobject.SharedObjectConfig) (*sobject.SOConfigChange, error) {
	if base.GetConfigChainSeqno() != 0 {
		return nil, nil
	}
	read, err := s.objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer read.Discard()
	entry, err := readSOConfigEntry(ctx, read, s.GetSharedObjectID(), base.GetConfigChainHash())
	if err != nil || entry == nil {
		return nil, err
	}
	hash, err := sobject.HashSOConfigChange(entry)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(hash, base.GetConfigChainHash()) {
		return nil, sobject.ErrConfigHistoryUnavailable
	}
	return entry, sobject.VerifyConfigChain([]*sobject.SOConfigChange{entry})
}

// ReadSharedObjectFullConfigHistory proves an imported object's original lineage.
func (s *SharedObject) ReadSharedObjectFullConfigHistory(ctx context.Context, target *sobject.SharedObjectConfig) ([]*sobject.SOConfigChange, error) {
	base, suffix, err := s.ReadSharedObjectConfigHistory(ctx, target)
	if err != nil {
		return nil, err
	}
	genesis, err := s.ReadSharedObjectGenesis(ctx, base)
	if err != nil {
		return nil, err
	}
	if genesis == nil {
		return nil, errors.Wrap(sobject.ErrConfigHistoryUnavailable, "the original Space replica must supply its signed history before cloud import")
	}
	return append([]*sobject.SOConfigChange{genesis}, suffix...), nil
}
