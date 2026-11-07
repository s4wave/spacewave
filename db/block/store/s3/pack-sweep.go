//go:build !tinygo

package block_store_s3

import (
	"context"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/pkg/errors"
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
