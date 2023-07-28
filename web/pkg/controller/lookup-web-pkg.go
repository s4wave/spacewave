package web_pkg_controller

import (
	"context"

	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	"github.com/aperturerobotics/controllerbus/directive"
	"golang.org/x/exp/slices"
)

// resolveLookupWebPkg returns a resolver for looking up a volume.
func (c *Controller) resolveLookupWebPkg(
	ctx context.Context,
	di directive.Instance,
	dir web_pkg.LookupWebPkg,
) (directive.Resolver, error) {
	// check if we can immediately reject this directive
	pkgID := dir.LookupWebPkgID()
	if len(c.webPkgIds) != 0 {
		if !slices.Contains(c.webPkgIds, pkgID) {
			return nil, nil
		}
	}

	// resolve by calling the getter func
	return directive.NewKeyedGetterResolver[string, web_pkg.LookupWebPkgValue](
		c.getter,
		pkgID,
	), nil
}
