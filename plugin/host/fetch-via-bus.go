package plugin_host

import (
	"context"

	"github.com/aperturerobotics/bldr/plugin"
	bldr_rpc "github.com/aperturerobotics/bldr/rpc"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// PluginFetchViaBusControllerID is the controller ID used for PluginFetchViaBus.
const PluginFetchViaBusControllerID = "plugin/fetch-via-bus"

// PluginFetchViaBusVersion is the controller version used for PluginFetchViaBus.
var PluginFetchViaBusVersion = semver.MustParse("0.0.1")

// PluginFetchViaBus implements the PluginFetch service.
type PluginFetchViaBus struct {
	le *logrus.Entry
	b  bus.Bus
}

// NewPluginFetchViaBus constructs a new PluginFetchViaBus implementation.
func NewPluginFetchViaBus(le *logrus.Entry, b bus.Bus) *PluginFetchViaBus {
	return &PluginFetchViaBus{le: le, b: b}
}

// NewPluginFetchViaBusController constructs a new controller resolving
// LookupRpcService with the FetchPluginViaBus service.
func NewPluginFetchViaBusController(le *logrus.Entry, b bus.Bus) *bldr_rpc.InvokerController {
	mux := srpc.NewMux()
	f := NewPluginFetchViaBus(le, b)
	plugin.SRPCRegisterPluginFetch(mux, f)
	return bldr_rpc.NewInvokerController(
		le,
		b,
		controller.NewInfo(
			PluginFetchViaBusControllerID,
			PluginFetchViaBusVersion,
			"FetchPlugin rpc to directive",
		),
		mux,
		nil,
	)
}

// FetchPlugin fetches a plugin by id.
func (f *PluginFetchViaBus) FetchPlugin(
	ctx context.Context,
	req *plugin.FetchPluginRequest,
) (*plugin.FetchPluginResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	pluginID := req.GetPluginId()
	f.le.Infof("plugin host requests fetching plugin: %s", pluginID)

	return ExFetchPlugin(ctx, f.b, pluginID, false)
}

// _ is a type assertion
var _ plugin.SRPCPluginFetchServer = ((*PluginFetchViaBus)(nil))
