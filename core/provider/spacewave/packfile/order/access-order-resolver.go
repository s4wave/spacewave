package order

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// AccessOrderPathResolver resolves a manifest file path to current block refs.
type AccessOrderPathResolver interface {
	ResolveAccessOrderPath(ctx context.Context, filesystem AccessOrderFilesystem, path string) ([]*block.BlockRef, bool, error)
}

// AccessOrderPathResolverFunc adapts a function into an AccessOrderPathResolver.
type AccessOrderPathResolverFunc func(ctx context.Context, filesystem AccessOrderFilesystem, path string) ([]*block.BlockRef, bool, error)

// ResolveAccessOrderPath resolves a manifest file path to current block refs.
func (f AccessOrderPathResolverFunc) ResolveAccessOrderPath(ctx context.Context, filesystem AccessOrderFilesystem, path string) ([]*block.BlockRef, bool, error) {
	return f(ctx, filesystem, path)
}
