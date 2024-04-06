package web_pkg_rpc_client

import (
	"context"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	web_pkg_rpc "github.com/aperturerobotics/bldr/web/pkg/rpc"
	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveLookupWebPkg resolves LookupWebPkg.
type resolveLookupWebPkg = directive.TransformResolver[web_pkg.LookupWebPkgValue]

// newResolveLookupWebPkg constructs a new LookupWebPkg resolver.
func newResolveLookupWebPkg(c *Controller, webPkgID string) *resolveLookupWebPkg {
	serviceID := c.serviceIdPrefix + webPkgID
	return directive.NewTransformResolver(
		c.bus,
		bifrost_rpc.NewLookupRpcClient(serviceID, c.cc.GetClientId()),
		func(ctx context.Context, val directive.AttachedValue) (web_pkg.LookupWebPkgValue, func(), bool, error) {
			client, ok := val.GetValue().(bifrost_rpc.LookupRpcClientValue)
			if !ok {
				return nil, nil, false, nil
			}

			accessClient := web_pkg_rpc.NewSRPCAccessWebPkgClientWithServiceID(client, serviceID)
			proxyWebPkg, err := NewRemoteWebPkg(ctx, webPkgID, accessClient)
			if err != nil {
				return nil, nil, false, err
			}

			var result web_pkg.WebPkg = proxyWebPkg
			return result, proxyWebPkg.Release, true, nil
		},
	)
}
