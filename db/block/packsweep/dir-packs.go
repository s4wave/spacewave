package packsweep

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile/identity"
)

// DirPacks reads packs from local copies in a directory, named by pack id,
// and from the next source for the packs it lacks or holds at another size.
type DirPacks struct {
	PackSource
	// Dir holds the copies.
	Dir string
	// Local counts the packs read from Dir; the snapshot reads several at
	// once.
	Local atomic.Int64
}

// ReadPackFile reads the local copy when it has the pack's size.
func (d *DirPacks) ReadPackFile(ctx context.Context, id string, size uint64) ([]byte, error) {
	// Refuse an id that could name a path outside the directory.
	if err := identity.ValidatePackID(id); err != nil {
		return nil, err
	}

	// Read the copy, falling back when it is absent or another size.
	f, err := os.Open(filepath.Join(d.Dir, id))
	if errors.Is(err, os.ErrNotExist) {
		return d.PackSource.ReadPackFile(ctx, id, size)
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if uint64(info.Size()) != size { //nolint:gosec // a file size is never negative.
		return d.PackSource.ReadPackFile(ctx, id, size)
	}

	// Read the matching copy whole.
	data := make([]byte, size)
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, errors.Wrap(err, "read local pack "+id)
	}
	d.Local.Add(1)
	return data, nil
}
