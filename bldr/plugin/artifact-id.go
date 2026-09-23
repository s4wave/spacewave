package bldr_plugin

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/hash"
)

// PluginArtifactID addresses immutable distribution and asset files.
// An empty manifest root follows the current plugin's files.
func PluginArtifactID(pluginID, manifestRoot string) string {
	if manifestRoot == "" {
		return pluginID
	}
	return pluginID + "/manifest/" + manifestRoot
}

// ParsePluginArtifactID validates a plugin file binding and its optional exact root.
func ParsePluginArtifactID(artifactID string, allowEmpty bool) (pluginID, manifestRoot string, err error) {
	pluginID, manifestRoot, pinned := strings.Cut(artifactID, "/manifest/")
	if err := ValidatePluginID(pluginID, allowEmpty && !pinned); err != nil {
		return "", "", err
	}
	if pinned {
		var root hash.Hash
		if err := root.ParseFromB58(manifestRoot); err != nil {
			return "", "", err
		}
		if err := root.Validate(); err != nil {
			return "", "", err
		}
	}
	return pluginID, manifestRoot, nil
}

// ParseHTTPPathPluginArtifact separates a plugin file binding from the file path.
// The manifest segment stays in the base URL, so relative module imports retain it.
func ParseHTTPPathPluginArtifact(httpPath string) (artifactID, suffix string, err error) {
	pluginID, suffix, err := ParseHTTPPathPluginID(httpPath)
	if err != nil {
		return "", "", err
	}
	manifestPath, pinned := strings.CutPrefix(suffix, "/manifest/")
	if !pinned {
		return pluginID, suffix, nil
	}
	root, filePath, hasPath := strings.Cut(manifestPath, "/")
	if !hasPath || root == "" {
		return "", "", errors.New("immutable plugin URL requires a file path")
	}
	artifactID = PluginArtifactID(pluginID, root)
	if _, _, err := ParsePluginArtifactID(artifactID, false); err != nil {
		return "", "", err
	}
	return artifactID, "/" + filePath, nil
}
