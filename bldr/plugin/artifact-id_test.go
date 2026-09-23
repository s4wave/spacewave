package bldr_plugin

import (
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

// TestPluginArtifactPaths retains the selected manifest through HTTP and FS RPC.
func TestPluginArtifactPaths(t *testing.T) {
	root, err := hash.Sum(hash.RecommendedHashType, []byte("module"))
	if err != nil {
		t.Fatal(err)
	}
	id := PluginArtifactID("colors", root.MarshalString())
	for _, file := range []string{"entry.mjs", "chunks/shared.mjs", "viewer.css"} {
		binding, path, err := ParseHTTPPathPluginArtifact(id + "/" + file)
		if err != nil || binding != id || path != "/"+file {
			t.Fatalf("file binding: %q %q %v", binding, path, err)
		}
		for _, fsID := range []string{PluginDistFsId(binding), PluginAssetsFsId(binding)} {
			if got, _, err := ValidatePluginUnixfsID(fsID, false); err != nil || got != id {
				t.Fatalf("filesystem binding: %q %v", got, err)
			}
		}
	}
	for _, path := range []string{"colors/manifest//entry.mjs", "colors/manifest/latest/entry.mjs", id} {
		if _, _, err := ParseHTTPPathPluginArtifact(path); err == nil {
			t.Errorf("invalid immutable path accepted: %s", path)
		}
	}
}
