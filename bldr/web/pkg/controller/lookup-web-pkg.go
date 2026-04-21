package web_pkg_controller

import (
	"slices"

	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveLookupWebPkg returns a resolver for looking up a volume.
func (c *Controller) resolveLookupWebPkg(dir web_pkg.LookupWebPkg) (directive.Resolver, error) {
	// check if we can immediately reject this directive
	pkgID := dir.LookupWebPkgID()
	if len(c.webPkgIds) != 0 {
		if !slices.Contains(c.webPkgIds, pkgID) {
			return nil, nil
		}
	}

	// resolve by calling the getter func
	return directive.NewKeyedGetterResolver(c.getter, pkgID), nil
}
