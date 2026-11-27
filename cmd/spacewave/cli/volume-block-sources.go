//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// s4dbBlocks reads raw, unverified block bytes from one read-only snapshot.
type s4dbBlocks struct {
	// tx holds the source snapshot until the close function discards it.
	tx kvtx.Tx
	// keys is the source's default volume key layout.
	keys *store_kvkey.KVKey
}

// GetStoredBlock reads raw block bytes, or returns nil when the snapshot lacks them.
func (s *s4dbBlocks) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Resolve the physical block key without opening the source as a Volume.
	key, err := ref.MarshalKey()
	if err != nil {
		return nil, err
	}
	data, found, err := s.tx.Get(ctx, s.keys.GetBlockKey(key))
	if err != nil || !found {
		return nil, err
	}
	return &block.StoredBlock{Data: data}, nil
}

// openS4DBBlocks opens a stopped source and holds one snapshot for the caller's stream.
// The caller must call closeSource to discard the snapshot before closing the file.
// Only the engine's reader-slot bookkeeping may write to this read-only source.
func openS4DBBlocks(ctx context.Context, path string) (_ *s4dbBlocks, closeSource func() error, err error) {
	// Release the engine on every failure before ownership reaches the caller.
	source, err := s4db.Open(path, s4db.Options{ReadOnly: true})
	if err != nil {
		return nil, nil, errors.Wrap(err, "open block source")
	}
	defer func() {
		if err != nil {
			err = stderrors.Join(err, errors.Wrap(source.Close(), "close block source"))
		}
	}()

	// Refuse a live writer before taking the stream's single snapshot.
	shared, err := source.Shared()
	if err != nil {
		return nil, nil, errors.Wrap(err, "check whether the source volume is stopped")
	}
	if shared {
		return nil, nil, errors.Errorf("%s is open in another process. Stop it first", path)
	}
	tx, err := source.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, errors.Wrap(err, "read block source")
	}
	return &s4dbBlocks{tx: tx, keys: store_kvkey.NewDefaultKVKey()}, func() error {
		tx.Discard()
		return source.Close()
	}, nil
}

// dirBlocks is a directory of blocks named by their B58 refs.
type dirBlocks string

// GetStoredBlock reads a regular block file, or returns nil when it is missing.
// A carrier travels between hosts, so a symlink would leave it without the bytes,
// even when the linked file hashes correctly, and a FIFO would block the run.
func (d dirBlocks) GetStoredBlock(_ context.Context, ref *block.BlockRef) (_ *block.StoredBlock, err error) {
	// Judge the name itself, so a symlink or a special file is refused unopened.
	path := filepath.Join(string(d), ref.MarshalString())
	named, err := os.Lstat(path) // #nosec G703 -- the operator-named directory joined with a B58 ref, which holds no separator.
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "stat block %s", path)
	}
	if !named.Mode().IsRegular() {
		return nil, errors.Errorf("block %s is not a regular file", path)
	}

	// Read the file that was judged: a name replaced between the two calls is refused.
	// #nosec G703 -- path is the name judged above.
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "open block %s", path)
	}
	defer func() { err = stderrors.Join(err, errors.Wrapf(file.Close(), "close block %s", path)) }()
	opened, err := file.Stat()
	if err != nil {
		return nil, errors.Wrapf(err, "stat block %s", path)
	}
	if !os.SameFile(named, opened) {
		return nil, errors.Errorf("block %s changed while it was opened", path)
	}

	// A block has no size limit here; the caller hashes whatever the file holds.
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, errors.Wrapf(err, "read block %s", path)
	}
	return &block.StoredBlock{Data: data}, nil
}

// _ is a type assertion.
var (
	_ packcopy.Source = (*s4dbBlocks)(nil)
	_ packcopy.Source = dirBlocks("")
)
