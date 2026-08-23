package space_http_export

import (
	"context"
	"io"

	"github.com/s4wave/spacewave/db/unixfs"
)

// exportZip writes a zip archive of the FSHandle tree to w.
func exportZip(ctx context.Context, w io.Writer, h *unixfs.FSHandle) error {
	return unixfs.WriteZipArchive(ctx, w, h, "")
}

// exportNamedZip writes one selected handle into a zip archive under the given root name.
func exportNamedZip(ctx context.Context, w io.Writer, h *unixfs.FSHandle, rootName string) error {
	return unixfs.WriteZipArchive(ctx, w, h, rootName)
}
