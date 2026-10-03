package space_http_export

import (
	"archive/zip"
	"context"
	"io"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
)

func exportBatchZip(ctx context.Context, w io.Writer, baseHandle *unixfs.FSHandle, relPaths []string) error {
	// Validate the selected descendant paths before opening the archive.
	normalizedPaths, err := normalizeBatchPaths(relPaths)
	if err != nil {
		return err
	}

	// Write each selected filesystem target into the batch archive.
	zw := zip.NewWriter(w)
	for _, relPath := range normalizedPaths {
		targetHandle, _, err := baseHandle.LookupPath(ctx, relPath)
		if err != nil {
			if targetHandle != nil {
				targetHandle.Release()
			}
			zw.Close()
			return errors.Wrap(err, "lookup batch path "+relPath)
		}

		if err := unixfs.WriteFSHandleToZip(ctx, zw, targetHandle, relPath); err != nil {
			targetHandle.Release()
			zw.Close()
			return err
		}
		targetHandle.Release()
	}

	return zw.Close()
}
