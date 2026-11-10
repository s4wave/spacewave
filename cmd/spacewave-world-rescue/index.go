//go:build !js && !wasip1 && !windows

package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
)

// openIndexView opens the pack index copy at path read-only without changing
// any of its files. A read-only SQLite connection still writes the -shm
// wal-index, so the view hard links the file and its -wal into scratch, which
// must be on the same file system, and SQLite keeps its own -shm there. The
// links are left in place and reused by a later run.
func openIndexView(ctx context.Context, path, scratch, table string) (store_kvtx_sqlite.Store, error) {
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		return nil, err
	}
	view := filepath.Join(scratch, filepath.Base(path))
	for _, suffix := range []string{"", "-wal"} {
		if err := linkInto(path+suffix, view+suffix, suffix == "-wal"); err != nil {
			return nil, err
		}
	}
	return store_kvtx_sqlite.OpenWithPragmas(ctx, view, table, store_kvtx_sqlite.Pragmas{ReadOnly: true})
}

// linkInto hard links src at dst, or checks that dst already is src. A
// missing src is skipped when optional.
func linkInto(src, dst string, optional bool) error {
	// Find src, skipping it when optional and missing.
	srcInfo, err := os.Stat(src)
	if err != nil {
		if optional && os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Accept an existing dst only when it is src.
	if dstInfo, err := os.Stat(dst); err == nil {
		if os.SameFile(srcInfo, dstInfo) {
			return nil
		}
		return errors.Errorf("%s exists and is not a link of %s; use a fresh --index-scratch", dst, src)
	}

	// Link src at dst.
	if err := os.Link(src, dst); err != nil {
		return errors.Wrap(err, "link the index into --index-scratch, which must be on the index's file system")
	}
	return nil
}
