package forge_lib_docker

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	forge_lib_util_inputfs "github.com/s4wave/spacewave/forge/lib/util/inputfs"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// storeOutputs stores each declared output in dir and sets it as the output of
// that name: a regular file as a blob, a directory as a UnixFS tree. An absent
// entry leaves its output unset; any other entry, or a file over
// block.MaxBlockSize, fails the execution.
func (c *Controller) storeOutputs(ctx context.Context, dir string) error {
	var values forge_value.ValueSlice
	for _, name := range c.conf.GetOutputs() {
		value, err := c.storeOutput(ctx, filepath.Join(dir, name))
		if err != nil {
			return errors.Wrapf(err, "output %q", name)
		}
		if value != nil {
			value.Name = name
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return c.handle.SetOutputs(ctx, values, false)
}

// storeOutput stores one output file or directory, returning nil when it is
// absent.
func (c *Controller) storeOutput(ctx context.Context, path string) (*forge_value.Value, error) {
	// Inspect the entry without following a link the container may have planted.
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		root, err := forge_lib_util_inputfs.StoreDir(ctx, c.handle, path)
		if err != nil {
			return nil, err
		}
		return forge_value.NewValueWithBlockRef("", root), nil
	}
	if !info.Mode().IsRegular() {
		return nil, errors.Errorf("is %s, not a regular file", info.Mode().Type())
	}
	if info.Size() > block.MaxBlockSize {
		return nil, errors.Errorf("is %d bytes, over the %d byte limit", info.Size(), block.MaxBlockSize)
	}

	// Store the stopped container's file as a blob behind a block reference.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	seed := forge_value.NewValueWithBlockRef("", nil)
	return forge_target.AccessValue(ctx, c.handle, seed, func(bcs *block.Cursor) error {
		_, err := blob.BuildBlob(ctx, info.Size(), f, bcs, nil)
		return err
	})
}
