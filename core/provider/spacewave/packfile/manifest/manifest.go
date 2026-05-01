package manifest

import (
	"bytes"
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"

	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
)

// metaLastPullSequenceKey is the kvtx key holding the last-seen monotonic
// sequence cursor returned by the cloud /sync/pull response. Pulls send
// strconv.FormatUint of this value as the wire cursor; a fresh client with
// no key seeds at 0 and receives the full pack list.
var metaLastPullSequenceKey = []byte("meta/lastPullSequence")

func manifestPackKey(packID string) []byte {
	shard := packID
	if len(shard) > 2 {
		shard = shard[len(shard)-2:]
	}
	return []byte("packs/" + shard + "/" + packID)
}

func manifestBloomKey(packID string) []byte {
	shard := packID
	if len(shard) > 2 {
		shard = shard[len(shard)-2:]
	}
	return []byte("pack_bloom/" + shard + "/" + packID)
}

// Manifest is a kvtx-backed persistent manifest of packfile entries.
type Manifest struct {
	store   kvtx.Store
	entries []*packfile.PackfileEntry
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
	tx, err := m.store.NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "creating read transaction")
	}
	defer tx.Discard()

	err = tx.ScanPrefix(ctx, []byte("packs/"), func(key, value []byte) error {
		entry := &packfile.PackfileEntry{}
		if err := entry.UnmarshalVT(value); err != nil {
			return errors.Wrap(err, "unmarshaling packfile entry")
		}
		if len(entry.GetBloomFilter()) == 0 {
			bloomData, found, err := tx.Get(ctx, manifestBloomKey(entry.GetId()))
			if err != nil {
				return errors.Wrap(err, "getting pack bloom filter")
			}
			if found {
				entry.BloomFilter = bytes.Clone(bloomData)
			}
		}
		m.entries = append(m.entries, entry)
		return nil
	})
	if err != nil {
		return err
	}
	slices.SortFunc(m.entries, func(a, b *packfile.PackfileEntry) int {
		return strings.Compare(a.GetId(), b.GetId())
	})
	return nil
}

// GetEntries returns a copy of the manifest entries.
func (m *Manifest) GetEntries() []*packfile.PackfileEntry {
	return slices.Clone(m.entries)
}

// GetLastPullSequence returns the last-seen monotonic pull sequence cursor
// from the store. A fresh client returns 0 so the next pull receives the
// full pack list.
func (m *Manifest) GetLastPullSequence(ctx context.Context) (uint64, error) {
	tx, err := m.store.NewTransaction(ctx, false)
	if err != nil {
		return 0, errors.Wrap(err, "creating read transaction")
	}
	defer tx.Discard()

	data, found, err := tx.Get(ctx, metaLastPullSequenceKey)
	if err != nil {
		return 0, errors.Wrap(err, "getting last pull sequence")
	}
	if !found {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "parsing last pull sequence")
	}
	return parsed, nil
}

// ApplyDelta applies entries and replacement events to the manifest and
// persists the highest pull sequence cursor.
func (m *Manifest) ApplyDelta(
	ctx context.Context,
	entries []*packfile.PackfileEntry,
	events []*packfile.PackReplacementEvent,
) error {
	if len(entries) == 0 && len(events) == 0 {
		return nil
	}

	tx, err := m.store.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "creating write transaction")
	}
	defer tx.Discard()

	next := make(map[string]*packfile.PackfileEntry, len(m.entries)+len(entries))
	for _, entry := range m.entries {
		if entry.GetId() == "" || entry.GetSupersededBy() != "" {
			continue
		}
		next[entry.GetId()] = entry.CloneVT()
	}
	for _, event := range events {
		for _, id := range event.GetReplacedPackIds() {
			delete(next, id)
			if err := tx.Delete(ctx, manifestPackKey(id)); err != nil {
				return errors.Wrap(err, "deleting replaced entry")
			}
			if err := tx.Delete(ctx, manifestBloomKey(id)); err != nil {
				return errors.Wrap(err, "deleting replaced bloom filter")
			}
		}
	}
	for _, entry := range entries {
		if entry.GetSupersededBy() != "" {
			delete(next, entry.GetId())
			if err := tx.Delete(ctx, manifestPackKey(entry.GetId())); err != nil {
				return errors.Wrap(err, "deleting superseded entry")
			}
			if err := tx.Delete(ctx, manifestBloomKey(entry.GetId())); err != nil {
				return errors.Wrap(err, "deleting superseded bloom filter")
			}
			continue
		}
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
		}
		next[entry.GetId()] = entry.CloneVT()
	}

	// Persist the maximum sequence across entries and replacement events as the
	// new pull cursor. Locally authored entries carry sequence 0, which never
	// advances the cursor.
	var maxSequence uint64
	for _, entry := range entries {
		if seq := entry.GetSequence(); seq > maxSequence {
			maxSequence = seq
		}
	}
	for _, event := range events {
		if seq := event.GetSequence(); seq > maxSequence {
			maxSequence = seq
		}
	}
	if maxSequence != 0 {
		if err := tx.Set(
			ctx,
			metaLastPullSequenceKey,
			[]byte(strconv.FormatUint(maxSequence, 10)),
		); err != nil {
			return errors.Wrap(err, "setting last pull sequence")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "applying manifest delta")
	}

	m.entries = sortedEntries(next)
	return nil
}

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

// IndexCache is a kvtx-backed cache for raw kvfile index-tail bytes.
type IndexCache struct {
	store kvtx.Store
}

// NewIndexCache creates a new IndexCache backed by the given store.
func NewIndexCache(store kvtx.Store) *IndexCache {
	return &IndexCache{store: store}
}

// Get returns cached raw index-tail bytes for a packfile.
func (c *IndexCache) Get(ctx context.Context, packID string) ([]byte, bool, error) {
	tx, err := c.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, false, errors.Wrap(err, "open index cache transaction")
	}
	defer tx.Discard()

	v, found, err := tx.Get(ctx, []byte("pack_idx/"+packID))
	if err != nil || !found {
		return nil, false, errors.Wrap(err, "get index cache entry")
	}

	return bytes.Clone(v), true, nil
}

// Set stores raw index-tail bytes for a packfile.
func (c *IndexCache) Set(ctx context.Context, packID string, data []byte) error {
	tx, err := c.store.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open index cache transaction")
	}
	defer tx.Discard()

	if err := tx.Set(ctx, []byte("pack_idx/"+packID), bytes.Clone(data)); err != nil {
		return errors.Wrap(err, "set index cache entry")
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit index cache entry")
	}
	return nil
}
