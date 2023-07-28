package web_pkg_static

import (
	"context"

	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// StaticWebPkg implements WebPkg with static information.
type StaticWebPkg struct {
	// info is the web pkg info
	info *web_pkg.WebPkgInfo
	// getFsHandle returns the root fs handle.
	getFsHandle func(ctx context.Context) (*unixfs.FSHandle, error)
}

// NewStaticWebPkg constructs a new static WebPkg.
func NewStaticWebPkg(
	info *web_pkg.WebPkgInfo,
	getFsHandle func(ctx context.Context) (*unixfs.FSHandle, error),
) (*StaticWebPkg, error) {
	return &StaticWebPkg{info: info, getFsHandle: getFsHandle}, nil
}

// GetId implements web_pkg.WebPkg.
func (p *StaticWebPkg) GetId() string {
	return p.info.GetId()
}

// GetInfo implements web_pkg.WebPkg.
func (p *StaticWebPkg) GetInfo(ctx context.Context) (*web_pkg.WebPkgInfo, error) {
	return p.info.CloneVT(), nil
}

// GetWebPkgFsHandle implements web_pkg.WebPkg.
func (p *StaticWebPkg) GetWebPkgFsHandle(ctx context.Context) (*unixfs.FSHandle, error) {
	if p.getFsHandle == nil {
		return nil, unixfs_errors.ErrInodeUnresolvable
	}
	return p.getFsHandle(ctx)
}

// _ is a type assertion
var _ web_pkg.WebPkg = ((*StaticWebPkg)(nil))
