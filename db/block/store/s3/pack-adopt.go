//go:build !tinygo

package block_store_s3

import (
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
	tail, err := s.verifyAdopted(ctx, entry, keys)
	if err != nil {
		return errors.Wrapf(err, "adopt pack %s", id)
	}

	// Register it, and list it as this store's own writes are.
	if err := s.index.addWritten(ctx, entry, keys); err != nil {
		return errors.Wrap(err, "index packfile")
	}
	s.index.cacheTail(ctx, id, tail)
	s.published(entry)
	return nil
}

// verifyAdopted reads the pack's index from the bucket and returns it, or an
// error unless it holds exactly keys.
func (s *PackStore) verifyAdopted(ctx context.Context, entry *packfile.PackfileEntry, keys [][]byte) ([]byte, error) {
	// Read the index at the end of the pack, sized by the entry.
	size := entry.GetSizeBytes()
	key := s.prefix + packDir + entry.GetId()
	read := func(ctx context.Context, off int64, length int) ([]byte, error) {
		return s.client.GetObjectRange(ctx, s.bucket, key, off, length)
	}
	tail, err := packfile.ReadIndexTail(ctx, size, packfile.IndexTailReadSize(entry.GetBlockCount()), read)
	if err != nil {
		return nil, err
	}

	// Every key the pack holds must be reported, and every reported key held.
	want := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		want[string(k)] = struct{}{}
	}
	var seen uint64
	err = packfile.ScanIndexTail(tail, size, func(e *kvfile.IndexEntry) error {
		if _, ok := want[string(e.GetKey())]; !ok {
			return errors.New("pack holds a block its report does not name")
		}
		seen++
		return nil
	})
	if err != nil {
		return nil, err
	}
	if seen != uint64(len(want)) || seen != entry.GetBlockCount() {
		return nil, errors.Errorf("pack holds %d blocks, its report names %d and its entry %d", seen, len(want), entry.GetBlockCount())
	}
	return tail, nil
}
