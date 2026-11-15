package bldr_plugin

import (
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
)

// ErrRelayRefused is returned to a caller whose call a plugin would have
// relayed to its host or to another plugin on someone else's behalf.
var ErrRelayRefused = errors.New("plugin refuses to relay the call")

// NewRelayRefusedResolver resolves a service lookup a plugin refuses to relay
// with ErrRelayRefused, so the caller fails at once rather than waiting on the
// bus for a resolver that never comes.
func NewRelayRefusedResolver(serverID string) directive.Resolver {
	return directive.NewFuncResolver(func(context.Context, directive.ResolverHandler) error {
		return errors.Wrapf(ErrRelayRefused, "server %q", serverID)
	})
}

// IsIncomingServerID reports whether serverID is one a plugin's muxes give a
// call that arrived from outside the plugin: "plugin-host/<id>" from the root
// mux, which serves the host, and "plugin/<caller>" or "plugin/<caller>/<id>"
// from PluginServer, which serves other plugins.
func IsIncomingServerID(serverID string) bool {
	return strings.HasPrefix(serverID, HostServerIDPrefix) || strings.HasPrefix(serverID, PluginServerIDPrefix)
}

// MayReachPluginRoot reports whether a call that arrives on the host bus under
// serverID may reach a plugin's root client, which carries the host's powers.
//
// Invariant: no call that originated in a plugin outside the host's set
// reaches another plugin's root client. On the host bus the identity of a call
// is the server ID the host itself assigned, never bytes the plugin sent, so
// the rule is an allow list that fails closed: a server ID in the "plugin/"
// namespace passes only when it is exactly "plugin/<X>" with X in
// hostPluginIDs. "plugin/<X>/<sub>" and "plugin/<X>" for any X outside the
// set are refused. A server ID outside that namespace was assigned by the host
// for its own calls and passes.
func MayReachPluginRoot(serverID string, hostPluginIDs []string) bool {
	pluginID, ok := strings.CutPrefix(serverID, PluginServerIDPrefix)
	if !ok {
		return true
	}
	return slices.Contains(hostPluginIDs, pluginID)
}
