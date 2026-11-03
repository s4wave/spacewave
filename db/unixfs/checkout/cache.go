package unixfs_checkout

import (
	"os"
	"path/filepath"

	"github.com/s4wave/spacewave/db/block"
)

// blockCache keeps the blocks a checkout read from its remotes in a
// directory beside the checkout, one file per block named by its ref, so a
// checkout cancelled or failed partway resumes from them. The checkout
// deletes it once it publishes its files, and Evict deletes one an abandoned
// checkout left. Two checkouts of one release may share it, since equal refs
// name equal bytes.
type blockCache struct {
	// dir holds the block files.
	dir string
}

// GetBlock reads ref's stored bytes, reporting false when absent.
func (c *blockCache) GetBlock(ref *block.BlockRef) ([]byte, bool) {
	data, err := os.ReadFile(c.path(ref))
	return data, err == nil
}

// put stores data as ref's bytes, writing a whole file or none.
func (c *blockCache) put(ref *block.BlockRef, data []byte) error {
	// Stage a complete block file before exposing it under its hash.
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(c.dir, ".block-")
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(file.Name(), c.path(ref))
	}
	if err != nil {
		_ = os.Remove(file.Name())
	}
	return err
}

// path is the file holding ref's bytes.
func (c *blockCache) path(ref *block.BlockRef) string {
	return filepath.Join(c.dir, ref.MarshalString())
}
