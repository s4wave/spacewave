package s4wave_worldop_registry

import (
	"net/url"
	"strings"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/net/hash"
)

// PinnedOperationID addresses one handler in an immutable plugin manifest.
// The identity lives in accepted World history and resolves without a current registration.
func PinnedOperationID(pluginID, manifestRoot, handlerID string) string {
	return "plugin-op/" + pluginID + "/" + manifestRoot + "/" + url.PathEscape(handlerID)
}

// ParsePinnedOperationID decodes an immutable operation identity.
// It rejects malformed identities before any plugin or transaction is acquired.
func ParsePinnedOperationID(id string) (pluginID, manifestRoot, handlerID string, ok bool) {
	parts := strings.SplitN(id, "/", 4)
	if len(parts) != 4 || parts[0] != "plugin-op" {
		return "", "", "", false
	}
	if err := bldr_plugin.ValidatePluginID(parts[1], false); err != nil {
		return "", "", "", false
	}
	var root hash.Hash
	if err := root.ParseFromB58(parts[2]); err != nil || root.Validate() != nil {
		return "", "", "", false
	}
	handler, err := url.PathUnescape(parts[3])
	if err != nil || handler == "" {
		return "", "", "", false
	}
	return parts[1], parts[2], handler, true
}
