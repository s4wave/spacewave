//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
)

// AdoptPack registers a pack another writer uploaded under this store's prefix,
// so a store that never lists the bucket can read it. Adopting a pack twice
// changes nothing.
//
// Invariant: a pack is registered only after its own index, read from the
// bucket, shows exactly keys. Registering it tells the write-back that these
// blocks are on the remote, which can drop the last local copy, so a report
// that is wrong, or a pack that is not there, must leave the blocks queued.
func (s *PackStore) AdoptPack(ctx context.Context, entry *packfile.PackfileEntry, keys [][]byte) error {
	return s.adopt(ctx, entry, func(ctx context.Context) ([][]byte, []byte, error) {
		tail, err := s.verifyAdopted(ctx, entry, keys)
		return keys, tail, err
	})
}

// AdoptBucketPack registers a pack another writer uploaded under this store's
// prefix when no copy of the pack is at hand to report its keys. The keys come
// from the pack's own index, read once from the bucket, which also serves the
// checks AdoptPack makes. Adopting a pack twice changes nothing.
//
// Invariant: a pack is registered only once the bucket holds an object at its
// key of exactly the entry's size, whose index lists exactly the entry's block
// count of distinct keys.
func (s *PackStore) AdoptBucketPack(ctx context.Context, entry *packfile.PackfileEntry) error {
	return s.adopt(ctx, entry, func(ctx context.Context) ([][]byte, []byte, error) {
		return s.verifyBucketPack(ctx, entry)
	})
}

// adopt registers entry with the keys and index tail verify returns, unless the
// store holds the pack already. verify runs only for a pack the store lacks.
func (s *PackStore) adopt(ctx context.Context, entry *packfile.PackfileEntry, verify func(ctx context.Context) ([][]byte, []byte, error)) error {
	// Adopting needs an index to register the pack in.
	if s.index == nil {
		return errors.New("pack store has no index")
	}
	id := entry.GetId()
	if err := identity.ValidatePackID(id); err != nil {
		return err
	}

	// Load the pack table first, as a write numbering a pack does.
	if err := s.loadEntries(ctx); err != nil {
		return err
	}
	held, err := s.HasPackfile(ctx, id)
	if err != nil || held {
		return err
	}

	// Check the pack against what it was reported to hold.
	keys, tail, err := verify(ctx)
	if err != nil {
		return errors.Wrapf(err, "adopt pack %s", id)
	}

	// Register it with the extents its index lists, and list it as this
	// store's own writes are.
	if err := s.index.addWritten(ctx, entry, keys, tail); err != nil {
		return errors.Wrap(err, "index packfile")
	}
	s.published(entry)
	return nil
}

// KeepsBlocks reports that the store never reclaims a block it holds.
func (s *PackStore) KeepsBlocks() bool {
	return s.keepPacks
}

// objectStatter reports an object's size without reading its body.
type objectStatter interface {
	HeadObject(ctx context.Context, bucket, key string) (*ObjectStat, error)
}

// verifyBucketPack checks that the bucket holds the pack at the entry's size,
// reads its index from the bucket and returns the keys it lists with the
// index, or an error unless it lists exactly the entry's block count.
func (s *PackStore) verifyBucketPack(ctx context.Context, entry *packfile.PackfileEntry) ([][]byte, []byte, error) {
	// The object must be the size the entry names, which locates its index.
	statter, ok := s.client.(objectStatter)
	if !ok {
		return nil, nil, errors.New("object store cannot report an object's size")
	}
	key := s.adoptedKey(entry)
	stat, err := statter.HeadObject(ctx, s.bucket, key)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "stat %s", key)
	}
	if stat.Size < 0 || uint64(stat.Size) != entry.GetSizeBytes() {
		return nil, nil, errors.Errorf("bucket object is %d bytes, its entry %d", stat.Size, entry.GetSizeBytes())
	}

	// Read the index once, take the keys it lists, and check them as a report.
	tail, err := s.readAdoptedTail(ctx, entry)
	if err != nil {
		return nil, nil, err
	}
	var keys [][]byte
	err = packfile.ScanIndexTail(tail, entry.GetSizeBytes(), func(e *kvfile.IndexEntry) error {
		keys = append(keys, bytes.Clone(e.GetKey()))
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if err := checkAdopted(tail, entry, keys); err != nil {
		return nil, nil, err
	}
	return keys, tail, nil
}

// verifyAdopted reads the pack's index from the bucket and returns it, or an
// error unless it holds exactly keys.
func (s *PackStore) verifyAdopted(ctx context.Context, entry *packfile.PackfileEntry, keys [][]byte) ([]byte, error) {
	tail, err := s.readAdoptedTail(ctx, entry)
	if err != nil {
		return nil, err
	}
	if err := checkAdopted(tail, entry, keys); err != nil {
		return nil, err
	}
	return tail, nil
}

// adoptedKey returns the bucket key of the pack entry names.
func (s *PackStore) adoptedKey(entry *packfile.PackfileEntry) string {
	return s.prefix + packDir + entry.GetId()
}

// readAdoptedTail reads the index at the end of the pack from the bucket,
// sized by the entry.
func (s *PackStore) readAdoptedTail(ctx context.Context, entry *packfile.PackfileEntry) ([]byte, error) {
	key := s.adoptedKey(entry)
	read := func(ctx context.Context, off int64, length int) ([]byte, error) {
		data, err := s.client.GetObjectRange(ctx, s.bucket, key, off, length)
		s.reads.downloaded(readIndex, len(data))
		return data, err
	}
	return packfile.ReadIndexTail(ctx, entry.GetSizeBytes(), packfile.IndexTailReadSize(entry.GetBlockCount()), read)
}

// checkAdopted returns an error unless the index tail of the pack entry names
// holds exactly keys, and as many as the entry's block count.
func checkAdopted(tail []byte, entry *packfile.PackfileEntry, keys [][]byte) error {
	// Every key the pack holds must be reported, and every reported key held.
	want := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		want[string(k)] = struct{}{}
	}
	var seen uint64
	err := packfile.ScanIndexTail(tail, entry.GetSizeBytes(), func(e *kvfile.IndexEntry) error {
		if _, ok := want[string(e.GetKey())]; !ok {
			return errors.New("pack holds a block its report does not name")
		}
		seen++
		return nil
	})
	if err != nil {
		return err
	}
	if seen != uint64(len(want)) || seen != entry.GetBlockCount() {
		return errors.Errorf("pack holds %d blocks, its report names %d and its entry %d", seen, len(want), entry.GetBlockCount())
	}
	return nil
}
