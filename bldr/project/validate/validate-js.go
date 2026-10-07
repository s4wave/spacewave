//go:build js

package bldr_project_validate

import (
	"context"
	"io/fs"

	"github.com/pkg/errors"
)

// Validate is unavailable in the browser, which cannot evaluate bldr.star.
func Validate(ctx context.Context, fsys fs.FS) (*Validation, error) {
	return nil, errors.New("plugin validation requires the native app")
}
