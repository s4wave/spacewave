package unixfs_checkout

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// Preparer shares explicit checkout operations. It starts a worker only for a
// call to Checkout and owns no permanent goroutine or daemon.
type Preparer struct {
	registry registry
}

// NewPreparer constructs an idle owner for concurrent checkout operations.
func NewPreparer() *Preparer {
	return &Preparer{registry: registry{ctx: context.Background(), jobs: make(map[string]*job)}}
}

// Checkout shares verified content work, stopping a worker after its last
// caller leaves and preserving partial fetched blocks for the next attempt.
func (p *Preparer) Checkout(ctx context.Context, request Request) error {
	// Canonicalize the target and refuse a directory outside its lease root.
	root, err := filepath.Abs(request.Root)
	if err != nil {
		return err
	}
	directory, err := filepath.Abs(request.Directory)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, directory)
	if err != nil {
		return err
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("checkout directory must be below its root")
	}
	if len(request.Roots) == 0 {
		return errors.New("checkout requires a content root")
	}

	// One explicit owner shares both foreground priority and destination work.
	attempt := &attempt{le: request.Logger, root: root, codec: request.Codec}
	return attempt.checkout(ctx, &p.registry, request.Fetch, request.Description, directory, request.Roots, request.Check)
}
