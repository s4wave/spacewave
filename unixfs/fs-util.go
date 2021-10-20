package unixfs

import (
	"context"
	"io/fs"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/pkg/errors"
)

// ReaddirAllToFileInfo calls readdir and generates FileInfo objects.
func ReaddirAllToFileInfo(ctx context.Context, h *FSHandle) ([]fs.FileInfo, error) {
	var children []string
	err := h.ReaddirAll(ctx, func(ent FSCursorDirent) error {
		children = append(children, ent.GetName())
		return nil
	})
	if err != nil {
		return nil, err
	}

	// lookup all children
	out := make([]fs.FileInfo, 0, len(children))
	for _, childName := range children {
		ch, err := h.Lookup(ctx, childName)
		if err == unixfs_errors.ErrNotExist {
			continue
		}
		if err != nil {
			return nil, errors.Wrapf(err, "lookup child %q", childName)
		}
		fi, err := ch.GetFileInfo(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "lookup child %q: file info", childName)
		}
		out = append(out, fi)
	}
	return out, nil
}
