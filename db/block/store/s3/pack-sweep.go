//go:build !tinygo

package block_store_s3

import (
	"context"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// maxReadPackBytes bounds the pack ReadPackObject reads into memory: the
// largest MaxPackBytes a store takes in practice, well over the policy's
// default.
const maxReadPackBytes = 1 << 30

// Packs returns every pack the store has registered, held ones included,
// sorted by id. With an index it reads the index's entries, and lists the
// bucket only when the index holds no pack.
func (s *PackStore) Packs(ctx context.Context) ([]*packfile.PackfileEntry, error) {
	// Publish every registered entry.
	if err := s.loadAll(ctx); err != nil {
		return nil, err
	}

	// Copy them out in id order.
	var out []*packfile.PackfileEntry
	s.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		out = slices.AppendSeq(slices.Collect(maps.Values(s.entries)), maps.Values(s.heldEntries))
	})
	slices.SortFunc(out, func(a, b *packfile.PackfileEntry) int { return strings.Compare(a.GetId(), b.GetId()) })
	return out, nil
}

// ReadPackFile reads the whole of pack id, which must be size bytes, in one
// streamed read.
func (s *PackStore) ReadPackFile(ctx context.Context, id string, size uint64) ([]byte, error) {
	return ReadPackObject(ctx, s.client, s.bucket, s.prefix, id, size)
}

// ReadPackObject reads the whole of pack id under prefix from client in one
// streamed read, refusing a pack that is not size bytes or exceeds
// maxReadPackBytes.
func ReadPackObject(ctx context.Context, client ObjectStore, bucket, prefix, id string, size uint64) ([]byte, error) {
	// Refuse a size no pack writer produces.
	if size > maxReadPackBytes {
		return nil, errors.Errorf("pack %s of %d bytes exceeds the %d byte limit", id, size, maxReadPackBytes)
	}

	// Read the body into one buffer of the pack's size.
	body, err := client.GetObject(ctx, bucket, prefix+packDir+id)
	if err != nil {
		return nil, errors.Wrap(err, "read pack "+id)
	}
	defer body.Close()
	data := make([]byte, size)
	if _, err := io.ReadFull(body, data); err != nil {
		return nil, errors.Wrap(err, "read pack "+id)
	}

	// A longer body is a different pack than its entry describes.
	var extra [1]byte
	if n, _ := body.Read(extra[:]); n != 0 {
		return nil, errors.Errorf("pack %s is longer than its %d byte entry", id, size)
	}
	return data, nil
}

// PackObjectKey returns the object key of pack id under prefix.
func PackObjectKey(prefix, id string) string {
	return prefix + packDir + id
}

// EntryObjectKey returns the object key of the entry of pack id under prefix.
func EntryObjectKey(prefix, id string) string {
	return prefix + entryDir + id
}

// PackHold is the sweep state of a pack and its last fallback read or lift.
type PackHold struct {
	// ID is the pack.
	ID string
	// State is the pack's state.
	State PackState
	// Since is when the pack entered its state, in Unix nanoseconds, or zero
	// for a registered pack the sweep never moved.
	Since int64
	// Event is when the last lift or miss read the pack, in Unix nanoseconds,
	// or zero when none did.
	Event int64
	// Key is the block key the event read, empty for a lift by the command.
	Key []byte
}

// Holds returns the sweep state of packs ids, in order, or of every pack the
// sweep moved when ids is empty, sorted by id. A pack the sweep never moved
// is registered with no record.
func (s *PackStore) Holds(ctx context.Context, ids []string) ([]PackHold, error) {
	// Read the q/ rows in one snapshot.
	if s.index == nil {
		return nil, errNoSweep
	}
	tx, err := s.index.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Read every row when no pack is named.
	decode := func(id string, value []byte) (PackHold, error) {
		hold, err := unmarshalPackHold(value)
		return PackHold{ID: id, State: hold.state, Since: hold.since, Event: hold.event, Key: hold.key}, err
	}
	var out []PackHold
	if len(ids) == 0 {
		err := tx.ScanPrefix(ctx, []byte(indexHoldPrefix), func(key, value []byte) error {
			hold, err := decode(string(key[len(indexHoldPrefix):]), value)
			out = append(out, hold)
			return err
		})
		return out, errors.Wrap(err, "read pack holds")
	}

	// Read the named packs' rows.
	out = make([]PackHold, len(ids))
	for i, id := range ids {
		value, ok, err := tx.Get(ctx, []byte(indexHoldPrefix+id))
		if err != nil {
			return nil, err
		}
		out[i] = PackHold{ID: id}
		if ok {
			if out[i], err = decode(id, value); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// LocatePacks returns the registered pack the index places each block in, in
// order, or "" for a block no registered pack holds by its row.
func (s *PackStore) LocatePacks(ctx context.Context, refs []*block.BlockRef) ([]string, error) {
	// Refuse a store without an index, and load its pack table.
	if s.index == nil {
		return nil, errors.New("pack store has no index")
	}
	if err := s.loadEntries(ctx); err != nil {
		return nil, err
	}

	// Read each block's row.
	keys := make([][]byte, len(refs))
	for i, ref := range refs {
		keys[i] = packfile.BlockKey(ref.GetHash())
	}
	return s.index.locate(ctx, keys, make([]bool, len(keys)))
}
