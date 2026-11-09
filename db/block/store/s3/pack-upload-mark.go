//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"sync"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// adoptUploads resolves the w/ rows a crash or a failed publish left: packs
// whose upload started and whose index never began. It reads each pack's
// index tail from the end of its object, one or two range reads per pack and
// none per block. A pack the bucket holds is indexed with the keys its tail
// lists, as a pack this store wrote is, after writing its entry object again
// so a listing keeps it; its marker goes in the first index slice. A pack the
// bucket lacks never finished its upload, and its marker is deleted. Any other
// failure leaves the marker for the next start and is logged, so a bucket
// error does not hold up the load. It returns the adopted entries.
//
// Call after the index's table is loaded, so the adopted packs take numbers no
// pack holds, and before any upload of this process, so every marker it finds
// is a stopped upload.
func (s *PackStore) adoptUploads(ctx context.Context) ([]*packfile.PackfileEntry, error) {
	// Read the markers.
	if s.index == nil || s.index.readOnly {
		return nil, nil
	}
	marked, err := s.index.uploads(ctx)
	if err != nil || len(marked) == 0 {
		return nil, err
	}

	// Resolve each one, a few at a time.
	var mtx sync.Mutex
	var adopted []*packfile.PackfileEntry
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(packUploads)
	for _, entry := range marked {
		eg.Go(func() error {
			ok, err := s.adoptUpload(egCtx, entry)
			if err != nil {
				if egCtx.Err() != nil {
					return egCtx.Err()
				}
				s.le.WithError(err).WithField("pack", entry.GetId()).Warn("keep the upload marker of a pack the index could not adopt")
				return nil
			}
			if ok {
				mtx.Lock()
				adopted = append(adopted, entry)
				mtx.Unlock()
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	s.le.WithFields(logrus.Fields{
		"marked":  len(marked),
		"adopted": len(adopted),
	}).Info("resolved the packfile uploads a stop interrupted")
	return adopted, nil
}

// adoptUpload resolves the marker of one pack, as adoptUploads describes, and
// reports whether it indexed the pack.
func (s *PackStore) adoptUpload(ctx context.Context, entry *packfile.PackfileEntry) (bool, error) {
	// A marker that does not decode names nothing to adopt. A marker never
	// shares a durable p/ row, which deletes it in the same transaction.
	id := entry.GetId()
	if entry.GetSizeBytes() == 0 {
		return false, s.index.dropUpload(ctx, id)
	}

	// Read the pack's index tail; a pack that is not there never uploaded.
	size := entry.GetSizeBytes()
	key := s.prefix + packDir + id
	tail, err := packfile.ReadIndexTail(ctx, size, packfile.IndexTailReadSize(entry.GetBlockCount()), func(ctx context.Context, off int64, length int) ([]byte, error) {
		return s.client.GetObjectRange(ctx, s.bucket, key, off, length)
	})
	if errors.Is(err, ErrNotFound) {
		return false, s.index.dropUpload(ctx, id)
	}
	if err != nil {
		return false, errors.Wrap(err, "read packfile index")
	}

	// Its keys must be the ones its entry counts.
	var keys [][]byte
	err = packfile.ScanIndexTail(tail, size, func(ie *kvfile.IndexEntry) error {
		keys = append(keys, bytes.Clone(ie.GetKey()))
		return nil
	})
	if err != nil {
		return false, err
	}
	if uint64(len(keys)) != entry.GetBlockCount() {
		return false, errors.Errorf("pack holds %d blocks, its entry %d", len(keys), entry.GetBlockCount())
	}

	// Write its entry, which the crash may have stopped, then index it.
	entryData, err := entry.MarshalVT()
	if err != nil {
		return false, err
	}
	if err := s.client.PutObject(ctx, s.bucket, s.prefix+entryDir+id, entryData, "application/octet-stream"); err != nil {
		return false, errors.Wrap(err, "write packfile entry")
	}
	if err := s.index.addWritten(ctx, entry, keys, tail); err != nil {
		// A stop after the first slice left the pack's p/ row in place of
		// the marker; the background indexer finishes it.
		if qerr := s.index.queueUnindexed(ctx, entry); qerr != nil {
			s.le.WithError(qerr).WithField("pack", id).Warn("queue a partly adopted pack for the indexer")
		}
		return false, errors.Wrap(err, "index packfile")
	}
	return true, nil
}
