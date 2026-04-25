// Package delta builds incremental packfile uploads from a writable block
// store by diffing against a CDN mirror view and chunking the delta into
// size-bounded kvfile packs.
package delta

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_kvfile "github.com/s4wave/spacewave/db/block/store/kvfile"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"

	alpha_cdn "github.com/s4wave/spacewave/core/cdn"
)

// MirrorUnion is a read-only block store that unions the kvfile packs under
// {mirrorDir}/{spaceID}/packs/{shard}/*.kvf. GetBlock / GetBlockExists try
// each pack in order and return the first hit. Write operations return
// block_store.ErrReadOnly. Callers must Close() the union when done so the
// backing file descriptors are released.
type MirrorUnion struct {
	stores []block.StoreOps
	files  []*os.File
	blocks uint64
}

// GetHashType returns 0 so the default hash type is used.
func (m *MirrorUnion) GetHashType() hash.HashType {
	return 0
}

// GetSupportedFeatures returns the native feature bitset.
func (m *MirrorUnion) GetSupportedFeatures() block.StoreFeature {
	return 0
}

// PutBlock rejects writes against the mirror.
func (m *MirrorUnion) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}

// PutBlockBatch rejects batched writes against the mirror.
func (m *MirrorUnion) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) error {
	if len(entries) == 0 {
		return nil
	}
	return block_store.ErrReadOnly
}

// PutBlockBackground rejects background writes against the mirror.
func (m *MirrorUnion) PutBlockBackground(_ context.Context, _ []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}

// GetBlock returns the first hit across the union of packs.
func (m *MirrorUnion) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	for _, s := range m.stores {
		data, found, err := s.GetBlock(ctx, ref)
		if err != nil {
			return nil, false, err
		}
		if found {
			return data, true, nil
		}
	}
	return nil, false, nil
}

// GetBlockExists returns true if any pack contains the block.
func (m *MirrorUnion) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	for _, s := range m.stores {
		ok, err := s.GetBlockExists(ctx, ref)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// GetBlockExistsBatch checks existence across the mirror.
func (m *MirrorUnion) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := m.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

// RmBlock rejects writes against the mirror.
func (m *MirrorUnion) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// StatBlock returns metadata for the block if any pack contains it.
func (m *MirrorUnion) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	for _, s := range m.stores {
		st, err := s.StatBlock(ctx, ref)
		if err != nil {
			return nil, err
		}
		if st != nil {
			return st, nil
		}
	}
	return nil, nil
}

// Flush has no buffered work for the read-only mirror.
func (m *MirrorUnion) Flush(_ context.Context) error {
	return nil
}

// BeginDeferFlush is a no-op for the read-only mirror.
func (m *MirrorUnion) BeginDeferFlush() {}

// EndDeferFlush is a no-op for the read-only mirror.
func (m *MirrorUnion) EndDeferFlush(_ context.Context) error {
	return nil
}

// BlockCount returns the sum of index entries across all packs. Blocks that
// appear in more than one pack are double-counted; this matches "how many
// kvfile entries back the mirror" rather than "how many unique refs".
func (m *MirrorUnion) BlockCount() uint64 {
	return m.blocks
}

// PackCount returns the number of kvfile packs backing the union.
func (m *MirrorUnion) PackCount() int {
	return len(m.stores)
}

// Close releases every open pack file.
func (m *MirrorUnion) Close() error {
	var firstErr error
	for _, f := range m.files {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	m.stores = nil
	m.files = nil
	return firstErr
}

// OpenMirrorUnion opens every =*.kvf= pack under
// ={mirrorDir}/{spaceID}/packs/= as a single read-only block.StoreOps union.
//
// Returns (nil, nil) when the per-space subdir, its =packs/= directory, or
// its set of kvfile packs is absent (fresh-Space case). If a
// =root.packedmsg= exists in the subdir, its embedded
// =CdnRootPointer.space_id= MUST equal =spaceID=; mismatch is a fatal error
// before any packs are opened.
//
// =le= may be nil to disable progress logging.
func OpenMirrorUnion(
	ctx context.Context,
	le *logrus.Entry,
	mirrorDir, spaceID string,
) (*MirrorUnion, error) {
	spaceDir := filepath.Join(mirrorDir, spaceID)
	info, err := os.Stat(spaceDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "stat mirror space dir")
	}
	if !info.IsDir() {
		return nil, errors.Errorf("mirror space path %q is not a directory", spaceDir)
	}

	if err := verifyMirrorRootPointer(spaceDir, spaceID); err != nil {
		return nil, err
	}

	packsDir := filepath.Join(spaceDir, "packs")
	if _, err := os.Stat(packsDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "stat mirror packs dir")
	}

	kvkey := store_kvkey.NewDefaultKVKey()
	u := &MirrorUnion{}
	success := false
	defer func() {
		if !success {
			_ = u.Close()
		}
	}()

	walkErr := filepath.WalkDir(packsDir, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".kvf") {
			return nil
		}
		f, openErr := os.Open(p)
		if openErr != nil {
			return errors.Wrapf(openErr, "open mirror pack %s", p)
		}
		rdr, buildErr := kvfile.BuildReaderWithFile(f)
		if buildErr != nil {
			_ = f.Close()
			return errors.Wrapf(buildErr, "build reader for %s", p)
		}
		u.stores = append(u.stores, block_store_kvfile.NewKvfileBlock(ctx, kvkey, rdr))
		u.files = append(u.files, f)
		u.blocks += rdr.Size()
		return nil
	})
	if walkErr != nil {
		return nil, errors.Wrap(walkErr, "walk mirror packs")
	}

	if len(u.stores) == 0 {
		return nil, nil
	}

	if le != nil {
		le.WithField("pack-files", len(u.stores)).
			WithField("mirror-blocks", u.blocks).
			Debug("mirror union opened")
	}
	success = true
	return u, nil
}

// verifyMirrorRootPointer enforces that a =root.packedmsg= present in the
// mirror subdir names the same space as =spaceID=. A missing file is fine
// (fresh mirror or packs-only case).
func verifyMirrorRootPointer(spaceDir, spaceID string) error {
	pmsgPath := filepath.Join(spaceDir, "root.packedmsg")
	body, err := os.ReadFile(pmsgPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return errors.Wrap(err, "read mirror root.packedmsg")
	}
	raw, ok := packedmsg.DecodePackedMessage(string(body))
	if !ok {
		return errors.New("decode mirror root.packedmsg: checksum mismatch or invalid base64")
	}
	ptr := &alpha_cdn.CdnRootPointer{}
	if err := ptr.UnmarshalVT(raw); err != nil {
		return errors.Wrap(err, "unmarshal mirror CdnRootPointer")
	}
	if ptr.GetSpaceId() != spaceID {
		return errors.Errorf(
			"mirror root.packedmsg space_id %q does not match expected space id %q",
			ptr.GetSpaceId(), spaceID,
		)
	}
	return nil
}

// _ is a type assertion
var _ block.StoreOps = ((*MirrorUnion)(nil))
