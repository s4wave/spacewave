// Package forge_lib_util_inputfs reads a Forge input that holds a UnixFS tree,
// and stores a directory as such a tree for an output.
package forge_lib_util_inputfs

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	unixfs_checkout "github.com/s4wave/spacewave/db/unixfs/checkout"
	forge_target "github.com/s4wave/spacewave/forge/target"
)

// Open returns a read-only handle on the UnixFS directory the named input
// holds. The handle reads through a clone of cs, so the handle is valid only
// within the storage access that produced cs. The caller releases the handle.
func Open(
	ctx context.Context,
	cs *bucket_lookup.Cursor,
	inputVals forge_target.InputMap,
	name string,
) (*unixfs.FSHandle, error) {
	// Resolve the input to the root of its tree.
	ref, err := inputVals.ObjectRef(name)
	if err != nil {
		return nil, err
	}

	// Read the tree through a cursor at its root with no writer.
	inputCs := cs.Clone()
	inputCs.SetRootRef(ref.GetRootRef())
	fs := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, inputCs, nil)
	handle, err := unixfs.NewFSHandle(fs)
	if err != nil {
		fs.Release()
		return nil, errors.Wrapf(err, "input %q", name)
	}
	return handle, nil
}

// Checkout writes the UnixFS directory the named input holds to dir on disk,
// replacing whatever dir held.
func Checkout(
	ctx context.Context,
	cs *bucket_lookup.Cursor,
	inputVals forge_target.InputMap,
	name, dir string,
) error {
	handle, err := Open(ctx, cs, inputVals, name)
	if err != nil {
		return err
	}
	defer handle.Release()
	return errors.Wrapf(unixfs_checkout.Checkout(ctx, dir, handle, nil), "check out input %q", name)
}

// ValidateName checks that name can identify an input's directory under a
// checkout root: non-empty, one path element, and not hidden.
func ValidateName(name string) error {
	if name == "" || strings.ContainsAny(name, "/\\") || strings.HasPrefix(name, ".") {
		return errors.Errorf("input must be a plain name: %q", name)
	}
	return nil
}

// CheckoutAll writes each named input to dir/<name> in the order given, all
// within one storage access of handle.
func CheckoutAll(
	ctx context.Context,
	handle forge_target.ExecControllerHandle,
	inputVals forge_target.InputMap,
	dir string,
	names []string,
) error {
	if len(names) == 0 {
		return nil
	}
	return handle.AccessStorage(ctx, nil, func(cs *bucket_lookup.Cursor) error {
		for _, name := range names {
			if err := Checkout(ctx, cs, inputVals, name, filepath.Join(dir, name)); err != nil {
				return err
			}
		}
		return nil
	})
}

// StoreDir writes the directory dir as a UnixFS tree into the Execution's
// World stage and returns its root, the value an output carries and a later
// input checks out. File contents stream through the blob builder. Symbolic
// links are skipped, never followed, so a tree written by a container cannot
// reach a host file. dir itself must be a directory, not a link.
func StoreDir(ctx context.Context, handle forge_target.ExecControllerHandle, dir string) (*block.BlockRef, error) {
	// Refuse a missing directory or a link in its place.
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.Errorf("is %s, not a directory", info.Mode().Type())
		}
		return nil, errors.Wrap(err, dir)
	}

	// Build the tree under the Execution's World stage and write it.
	var root *block.BlockRef
	err := handle.AccessStorage(ctx, nil, func(cs *bucket_lookup.Cursor) error {
		// Import the directory, then write its blocks and keep the root.
		btx, bcs := cs.BuildTransactionAtRef(nil, nil)
		if err := unixfs_block.CreateFromFS(ctx, bcs, os.DirFS(dir), handle.GetTimestamp()); err != nil {
			return err
		}
		var err error
		root, _, err = btx.Write(ctx, true)
		return err
	})
	return root, err
}
