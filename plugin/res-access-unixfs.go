package bldr_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	unixfs_rpc "github.com/aperturerobotics/hydra/unixfs/rpc"
	unixfs_rpc_client "github.com/aperturerobotics/hydra/unixfs/rpc/client"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// ParsePluginUnixfsID parses a unixfs ID and returns the plugin ID and matched prefix.
// Returns the plugin ID (which may be empty for current plugin) and the prefix.
// Returns empty matchedPrefix if no valid prefix is found.
//
// Matches UnixFS IDs like:
// - plugin-dist: matches the current plugin dist fs
// - plugin-assets: matches the current plugin assets fs
// - plugin-dist/{plugin-id}: matches the plugin dist fs for plugin-id
// - plugin-assets/{plugin-id}: matches the plugin assets fs for plugin-id
func ParsePluginUnixfsID(unixfsID string) (pluginID string, matchedPrefix string) {
	pluginID, matchedPrefix = srpc.CheckStripPrefix(unixfsID, []string{PluginDistFsIdPrefix, PluginAssetsFsIdPrefix})
	if matchedPrefix == "" {
		pluginID = ""
		if unixfsID == PluginDistFsId("") {
			matchedPrefix = PluginDistFsIdPrefix
		} else if unixfsID == PluginAssetsFsId("") {
			matchedPrefix = PluginAssetsFsIdPrefix
		}
	}
	return pluginID, matchedPrefix
}

// ValidatePluginUnixfsID validates a plugin unixfs ID and returns the plugin ID and matched prefix.
// If allowEmpty is true, allows empty plugin ID which refers to current plugin.
// Returns an error if the unixfsID is invalid.
func ValidatePluginUnixfsID(unixfsID string, allowEmpty bool) (pluginID string, matchedPrefix string, err error) {
	if unixfsID == "" {
		return "", "", errors.New("unixfs id must be set")
	}

	pluginID, matchedPrefix = ParsePluginUnixfsID(unixfsID)
	if matchedPrefix == "" {
		return "", "", errors.New("unixfs id prefix must be plugin-dist or plugin-assets")
	}

	// Validate plugin ID
	if err := ValidatePluginID(pluginID, allowEmpty); err != nil {
		return "", "", err
	}

	return pluginID, matchedPrefix, nil
}

// ResolveAccessUnixfs resolves a AccessUnixfs directive with another plugin.
//
// Resolves unixfs IDs like:
//   - plugin-dist/{plugin-id}
//   - plugin-assets/{plugin-id}
//
// Returns nil, nil if the service ID does not match any of the known prefixes.
// Returns an error if the plugin id is invalid.
func ResolveAccessUnixfs(ctx context.Context, dir unixfs_access.AccessUnixFS, h LookupRpcClientHandler) (directive.Resolver, error) {
	unixfsID := dir.AccessUnixFSID()
	if unixfsID == "" {
		return nil, nil
	}

	// check if the unixfs ID matches one of the known prefixes.
	pluginID, matchedPrefix := ParsePluginUnixfsID(unixfsID)
	if matchedPrefix == "" {
		// ignore
		return nil, nil
	}
	_ = pluginID

	// AccessUnixFSFunc is a function to access a UnixFS.
	// Optionally pass a released function that may be called when the handle was released.
	// Returns a release function.
	// TODO: move this to a common place in unixfs_access or unixfs_rpc_client
	var accessFunc unixfs_access.AccessUnixFSValue = func(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
		pluginHostClient, relFunc, err := h.WaitPluginHostClient(ctx, released)
		if err != nil {
			return nil, nil, err
		}

		pluginHostSvcClient := NewSRPCPluginHostClient(pluginHostClient)
		fsClient := rpcstream.NewRpcStreamClient(pluginHostSvcClient.PluginFsRpc, unixfsID, true)
		fsCursorSvcClient := unixfs_rpc.NewSRPCFSCursorServiceClient(fsClient)
		fsHandle, err := unixfs_rpc_client.BuildFSHandle(ctx, fsCursorSvcClient)
		if err != nil {
			if relFunc != nil {
				relFunc()
			}
			return nil, nil, err
		}

		return fsHandle, func() {
			fsHandle.Release()
			if relFunc != nil {
				relFunc()
			}
		}, nil
	}

	return directive.NewValueResolver([]unixfs_access.AccessUnixFSValue{accessFunc}), nil
}
