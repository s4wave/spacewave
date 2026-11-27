package bldr_plugin

import (
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
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

// webWorkerServerIDPrefix prefixes the server ID the web runtime gives a
// worker's calls on the host bus: "web-worker/<worker ID>", where a plugin's
// worker ID is "plugin/<X>" or "plugin/<X>/<execution key>".
const webWorkerServerIDPrefix = "web-worker/"

// MayReachPluginRoot reports whether a call that arrives on the host bus under
// serverID may reach a plugin's root client, which carries the host's powers.
//
// Invariant: no call that originated in a plugin outside the host's set
// reaches another plugin's root client. The rule is an allow list keyed on the
// plugin the server ID identifies, and the rule fails closed:
//
//   - "plugin/<X>" passes only with X in hostPluginIDs. "plugin/<X>/<sub>" is
//     refused.
//   - "web-worker/plugin/<X>" and "web-worker/plugin/<X>/<key>" pass only with
//     X in hostPluginIDs. Any other "web-worker/" ID is refused.
//   - Any other server ID passes: the host identifies its own calls ("",
//     "plugin-host/<id>"), and "web-view/<id>" carries no owning plugin, while
//     the app's own views need plugin roots.
//
// Limit: where the web runtime runs (a browser, an Electron renderer, devtool
// web) a worker's or view's ID is the connecting script's own claim, so the
// check stops a plugin that identifies itself honestly, not a script that
// claims a host plugin's ID or a view's. Only native plugin processes get
// host-assigned IDs.
func MayReachPluginRoot(serverID string, hostPluginIDs []string) bool {
	if workerID, ok := strings.CutPrefix(serverID, webWorkerServerIDPrefix); ok {
		pluginID, ok := strings.CutPrefix(workerID, PluginServerIDPrefix)
		pluginID, _, _ = strings.Cut(pluginID, "/")
		return ok && pluginID != "" && slices.Contains(hostPluginIDs, pluginID)
	}
	pluginID, ok := strings.CutPrefix(serverID, PluginServerIDPrefix)
	if !ok {
		return true
	}
	return slices.Contains(hostPluginIDs, pluginID)
}

// IsWebWorkerServerID reports whether serverID identifies a web worker's calls.
func IsWebWorkerServerID(serverID string) bool {
	return strings.HasPrefix(serverID, webWorkerServerIDPrefix)
}

// NewHostRootResolver guards res, a resolver that hands out a plugin's root
// client or the host's powers on b, with MayReachPluginRoot against the host
// set of b's root host scheduler.
//
// A server ID that passes with an empty host set passes with every set, so it
// resolves with res at once. Any other waits for the root host scheduler on b
// and is refused with ErrRelayRefused unless the scheduler's set admits it:
// with no host set yet the lookup reaches nothing.
func NewHostRootResolver(b bus.Bus, serverID string, res directive.Resolver) directive.Resolver {
	if MayReachPluginRoot(serverID, nil) {
		return res
	}
	return directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		// Wait for the host set, then refuse a server ID outside it.
		scheduler, err := waitHostPluginScheduler(ctx, b)
		if err != nil {
			return err
		}
		if !MayReachPluginRoot(serverID, scheduler.GetHostPluginIds()) {
			return NewRelayRefusedResolver(serverID).Resolve(ctx, handler)
		}
		return res.Resolve(ctx, handler)
	})
}
