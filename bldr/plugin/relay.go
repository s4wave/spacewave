package bldr_plugin

import (
	"context"
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
