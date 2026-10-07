package manifest

import (
	"bytes"
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
)

// metaLastPullSequenceKey is the kvtx key holding the last-seen monotonic
// sequence cursor returned by the cloud /sync/pull response. Pulls send
// strconv.FormatUint of this value as the wire cursor; a fresh client with
// no key seeds at 0 and receives the full pack list.
var metaLastPullSequenceKey = []byte("meta/lastPullSequence")

// manifestPackKey addresses the durable entry by ID.
func manifestPackKey(packID string) []byte {
	shard := packID
	if len(shard) > 2 {
		shard = shard[len(shard)-2:]
	}
	return []byte("packs/" + shard + "/" + packID)
}

// manifestBloomKey addresses the separately stored bloom bytes.
func manifestBloomKey(packID string) []byte {
	shard := packID
	if len(shard) > 2 {
		shard = shard[len(shard)-2:]
	}
	return []byte("pack_bloom/" + shard + "/" + packID)
}

// indexCacheKey is the kvtx key of the cached raw index tail of a packfile.
func indexCacheKey(packID string) []byte {
	return []byte("pack_idx/" + packID)
}

// deletePack removes the entry, bloom filter, and cached index tail of a
// packfile that left the active manifest.
func deletePack(ctx context.Context, tx kvtx.Tx, packID string) error {
	for _, key := range [][]byte{manifestPackKey(packID), manifestBloomKey(packID), indexCacheKey(packID)} {
		if err := tx.Delete(ctx, key); err != nil {
			return errors.Wrapf(err, "deleting packfile %s", packID)
		}
	}
	return nil
}

// Manifest is a kvtx-backed persistent manifest of packfile entries. It is safe
// for concurrent use; deltas apply one at a time.
type Manifest struct {
	// store persists entries, bloom filters, and the pull cursor.
	store kvtx.Store
	// mtx guards entries and serializes ApplyDelta.
	mtx sync.RWMutex
	// entries owns accepted immutable entries by pack ID.
	entries map[string]*packfile.PackfileEntry
}

// New creates a new Manifest, loading existing entries from the store.
func New(ctx context.Context, store kvtx.Store) (*Manifest, error) {
	m := &Manifest{store: store}
	if err := m.loadEntries(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

// loadEntries reads all entries from the store with the packs/ prefix.
func (m *Manifest) loadEntries(ctx context.Context) error {
	// Read every stored entry in one transaction.
	var entries []*packfile.PackfileEntry
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return m.store.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Collect the entries of this attempt only.
			attemptEntries := make([]*packfile.PackfileEntry, 0)
			err := tx.ScanPrefix(ctx, []byte("packs/"), func(key, value []byte) error {
				// Decode the entry, then attach its separately stored bloom filter.
				entry := &packfile.PackfileEntry{}
				if err := entry.UnmarshalVT(value); err != nil {
					return errors.Wrap(err, "unmarshaling packfile entry")
				}
				bloomData, found, err := tx.Get(ctx, manifestBloomKey(entry.GetId()))
				if err != nil {
					return errors.Wrap(err, "getting pack bloom filter")
				}
				if found {
					entry.BloomFilter = bytes.Clone(bloomData)
				}
				attemptEntries = append(attemptEntries, entry)
				return nil
			})
			if err != nil {
				return err
			}
			entries = attemptEntries
			return nil
		},
	)
	if err != nil {
		return errors.Wrap(err, "loading manifest entries")
	}

	// Index them by pack ID.
	m.entries = make(map[string]*packfile.PackfileEntry, len(entries))
	for _, entry := range entries {
		m.entries[entry.GetId()] = entry
	}
	return nil
}

// GetEntries returns a copy of the manifest entries.
func (m *Manifest) GetEntries() []*packfile.PackfileEntry {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return sortedEntries(m.entries)
}

// GetLastPullSequence returns the last-seen monotonic pull sequence cursor
// from the store. A fresh client returns 0 so the next pull receives the
// full pack list.
func (m *Manifest) GetLastPullSequence(ctx context.Context) (uint64, error) {
	var sequence uint64
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return m.store.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// A missing cursor reads as zero.
			data, found, err := tx.Get(ctx, metaLastPullSequenceKey)
			if err != nil {
				return errors.Wrap(err, "getting last pull sequence")
			}
			if !found {
				sequence = 0
				return nil
			}

			// Parse the stored decimal cursor.
			parsed, err := strconv.ParseUint(string(data), 10, 64)
			if err != nil {
				return errors.Wrap(err, "parsing last pull sequence")
			}
			sequence = parsed
			return nil
		},
	)
	if err != nil {
		return 0, errors.Wrap(err, "creating read transaction")
	}
	return sequence, nil
}

// advancePullSequence atomically preserves the greatest accepted server cursor.
func advancePullSequence(ctx context.Context, tx kvtx.Tx, sequence uint64) error {
	// Skip a zero cursor and one behind the stored cursor.
	if sequence == 0 {
		return nil
	}
	data, found, err := tx.Get(ctx, metaLastPullSequenceKey)
	if err != nil {
		return err
	}
	if found {
		previous, err := strconv.ParseUint(string(data), 10, 64)
		if err != nil {
			return err
		}
		if previous >= sequence {
			return nil
		}
	}
	return tx.Set(ctx, metaLastPullSequenceKey, []byte(strconv.FormatUint(sequence, 10)))
}

// ApplyDelta applies entries and replacement events to the manifest and
// advances the pull cursor to cursor in the same transaction. A zero cursor
// leaves it unchanged. A locally authored entry, which has no sequence, never
// overwrites a pulled entry for the same pack.
func (m *Manifest) ApplyDelta(
	ctx context.Context,
	entries []*packfile.PackfileEntry,
	events []*packfile.PackReplacementEvent,
	cursor uint64,
) error {
	// Skip a delta that changes neither the entries nor the cursor.
	if len(entries) == 0 && len(events) == 0 && cursor == 0 {
		return nil
	}

	// Commit the changed entries and the cursor in one store transaction.
	m.mtx.Lock()
	defer m.mtx.Unlock()
	var changed map[string]*packfile.PackfileEntry
	err := kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return m.store.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Stage only changed IDs. Nil represents a committed deletion.
			changed = make(map[string]*packfile.PackfileEntry, len(entries))
			for _, event := range events {
				for _, id := range event.GetReplacedPackIds() {
					changed[id] = nil
					if err := deletePack(ctx, tx, id); err != nil {
						return err
					}
				}
			}
			for _, entry := range entries {
				// Keep the accepted server descriptor ahead of stale local metadata.
				previous, touched := changed[entry.GetId()]
				if !touched {
					previous = m.entries[entry.GetId()]
				}
				if entry.GetSequence() < previous.GetSequence() {
					continue
				}

				// Remove an accepted tombstone together with its index and bloom.
				if entry.IsSuperseded() {
					changed[entry.GetId()] = nil
					if err := deletePack(ctx, tx, entry.GetId()); err != nil {
						return err
					}
					continue
				}

				// Evict the cached index when the accepted pack contents change.
				if previous != nil && (previous.GetSizeBytes() != entry.GetSizeBytes() || previous.GetBlockCount() != entry.GetBlockCount()) {
					if err := tx.Delete(ctx, indexCacheKey(entry.GetId())); err != nil {
						return errors.Wrap(err, "deleting changed pack index")
					}
				}

				// Encode the accepted descriptor without its separate bloom bytes.
				storedEntry := entry.CloneVT()
				storedEntry.BloomFilter = nil

				data, err := storedEntry.MarshalVT()
				if err != nil {
					return errors.Wrap(err, "marshaling entry")
				}
				if err := tx.Set(ctx, manifestPackKey(entry.GetId()), data); err != nil {
					return errors.Wrap(err, "putting entry")
				}
				if len(entry.GetBloomFilter()) != 0 {
					if err := tx.Set(
						ctx,
						manifestBloomKey(entry.GetId()),
						bytes.Clone(entry.GetBloomFilter()),
					); err != nil {
						return errors.Wrap(err, "putting bloom filter")
					}
				} else if err := tx.Delete(ctx, manifestBloomKey(entry.GetId())); err != nil {
					return err
				}
				changed[entry.GetId()] = entry.CloneVT()
			}

			return errors.Wrap(advancePullSequence(ctx, tx, cursor), "advance pull sequence")
		},
	)
	if err != nil {
		return errors.Wrap(err, "applying manifest delta")
	}

	// Publish the changes only after the durable transaction succeeds.
	for id, entry := range changed {
		if entry == nil {
			delete(m.entries, id)
			continue
		}
		m.entries[id] = entry
	}
	return nil
}

// GetEntry returns the accepted immutable entry, or nil after removal.
func (m *Manifest) GetEntry(id string) *packfile.PackfileEntry {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return m.entries[id]
}

// sortedEntries materializes an explicitly requested complete snapshot.
func sortedEntries(entries map[string]*packfile.PackfileEntry) []*packfile.PackfileEntry {
	out := make([]*packfile.PackfileEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	slices.SortFunc(out, func(a, b *packfile.PackfileEntry) int {
		return strings.Compare(a.GetId(), b.GetId())
	})
	return out
}
