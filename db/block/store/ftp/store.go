//go:build !tinygo

package block_store_ftp

import (
	"context"
	"encoding/hex"
	"path"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// ErrAppendOnly is returned by RmBlock: the store never deletes a block.
var ErrAppendOnly = errors.New("the FTP block store never deletes a block")

// Store is one block store's files on the server, one file per block.
//
// The block with hash h is the file {storeID}/blocks/{d0}/{d1}/{h} under the
// root, where d0 and d1 are the first two bytes of the digest in hex and h is
// the hash string, so no directory holds more than a few thousand files of a
// large store. The file holds the encoded block.BlockObject: the bytes and
// refs of the block.
type Store struct {
	le     *logrus.Entry
	client *Client
	// dir is the store's directory under the root.
	dir string
}

// NewStore builds the store of storeID's files. The store owns client and
// closes it on Close.
func NewStore(le *logrus.Entry, client *Client, storeID string) (*Store, error) {
	if storeID == "" || storeID == "." || storeID == ".." || strings.ContainsAny(storeID, "/\\") {
		return nil, errors.Errorf("block store id %q is not a file name", storeID)
	}
	return &Store{le: le, client: client, dir: storeID}, nil
}

// Close closes the client.
func (s *Store) Close() {
	s.client.Close()
}

// GetHashType returns 0 to use the default hash type.
func (s *Store) GetHashType() hash.HashType {
	return 0
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (s *Store) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeature_STORE_FEATURE_UNKNOWN
}

// BeginReadOperation returns the store as the scoped read handle.
func (s *Store) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// PutBlock stores one block.
func (s *Store) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Build the reference and store the block as a batch of one.
	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	entry := &block.PutBatchEntry{Ref: ref, Data: data, Refs: opts.GetRefs(), RefsKnown: opts.GetRefsKnown()}
	if _, err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{entry}); err != nil {
		return nil, false, err
	}
	return ref, false, nil
}

// PutBlockBatch stores each block as its own file, as many at once as the
// client has connections. Tombstones are rejected, and so is a block whose
// refs are unknown, since the file would record it as a leaf. No block is
// reported as existing.
func (s *Store) PutBlockBatch(ctx context.Context, batch []*block.PutBatchEntry) ([]bool, error) {
	// Encode every block before storing any.
	names := make([]string, len(batch))
	files := make([][]byte, len(batch))
	for i, entry := range batch {
		if entry.Tombstone {
			return nil, ErrAppendOnly
		}
		if !entry.RefsKnown {
			return nil, errors.Wrapf(block.ErrRefsUnknown, "store block %s", entry.Ref.MarshalString())
		}
		name, err := s.blockName(entry.Ref)
		if err != nil {
			return nil, err
		}
		file, err := block.EncodeBlockObject(entry.Data, entry.Refs)
		if err != nil {
			return nil, err
		}
		names[i], files[i] = name, file
	}

	// Store the files over the pooled connections.
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(s.client.MaxConns())
	for i := range batch {
		eg.Go(func() error {
			return s.client.Put(egCtx, names[i], files[i])
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return make([]bool, len(batch)), nil
}

// GetBlock looks up a block in the store.
func (s *Store) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored, err := s.GetStoredBlock(ctx, ref)
	return stored.GetData(), stored != nil, err
}

// GetStoredBlock reads the block with its refs and verifies its bytes against
// ref. Returns nil if the block is not found.
func (s *Store) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Read the file and decode it.
	name, err := s.blockName(ref)
	if err != nil {
		return nil, err
	}
	file, found, err := s.client.Get(ctx, name)
	if err != nil || !found {
		return nil, err
	}
	stored, err := block.DecodeBlockObject(file)
	if err != nil {
		return nil, errors.Wrap(err, name)
	}
	if err := ref.VerifyData(stored.Data, true); err != nil {
		return nil, errors.Wrap(err, name)
	}
	return stored, nil
}

// GetBlockExists checks if a block exists.
func (s *Store) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	name, err := s.blockName(ref)
	if err != nil {
		return false, err
	}
	_, found, err := s.client.Size(ctx, name)
	return found, err
}

// GetBlockExistsBatch checks the store for each block, as many at once as the
// client has connections. Each check is one SIZE command, which needs no data
// connection.
func (s *Store) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Check each block over the pooled connections.
	exists := make([]bool, len(refs))
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(s.client.MaxConns())
	for i, ref := range refs {
		eg.Go(func() error {
			var err error
			exists[i], err = s.GetBlockExists(egCtx, ref)
			return err
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return exists, nil
}

// StatBlock returns the block's metadata, or nil if the block is not found.
// The file's size includes its refs, so the stat reads the block.
func (s *Store) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	stored, err := s.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: int64(len(stored.Data))}, nil
}

// RmBlock returns ErrAppendOnly: the store never deletes a block.
func (s *Store) RmBlock(context.Context, *block.BlockRef) error {
	return ErrAppendOnly
}

// Sync reports always-durable: each file is renamed into place before
// PutBlockBatch returns. FTP has no fsync, so durability past that point is
// the server's.
func (s *Store) Sync(context.Context) (bool, error) {
	return true, nil
}

// Reclaim drops nothing and calls neither fence nor live: every block stays.
func (s *Store) Reclaim(
	context.Context,
	func(context.Context) error,
	func(context.Context, []*block.BlockRef) ([]bool, error),
) (time.Time, error) {
	return time.Time{}, nil
}

// blockName returns the file name of a block under the root.
func (s *Store) blockName(ref *block.BlockRef) (string, error) {
	// Name the file by the digest fan-out and the hash string.
	if ref.GetEmpty() {
		return "", block.ErrEmptyBlockRef
	}
	h := ref.GetHash()
	if err := h.Validate(); err != nil {
		return "", err
	}
	digest := h.GetHash()
	return path.Join(s.dir, "blocks", hex.EncodeToString(digest[:1]), hex.EncodeToString(digest[1:2]), h.MarshalString()), nil
}

// _ is a type assertion
var _ block.StoreOps = (*Store)(nil)
