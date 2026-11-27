package bldr_plugin

import (
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

// TestPluginArtifactPaths retains the selected manifest through HTTP and FS RPC.
func TestPluginArtifactPaths(t *testing.T) {
	// Compute a manifest root hash to pin the plugin artifact.
	root, err := hash.Sum(hash.RecommendedHashType, []byte("module"))
	if err != nil {
		t.Fatal(err)
	}
	id := PluginArtifactID("colors", root.MarshalString())

	// Parse each file path below the pinned artifact ID.
	for _, file := range []string{"entry.mjs", "chunks/shared.mjs", "viewer.css"} {
		// The HTTP binding keeps the manifest root and yields the file path.
		binding, path, err := ParseHTTPPathPluginArtifact(id + "/" + file)
		if err != nil || binding != id || path != "/"+file {
			t.Fatalf("file binding: %q %q %v", binding, path, err)
		}

		// The dist and assets filesystem bindings resolve to the same root.
		for _, fsID := range []string{PluginDistFsId(binding), PluginAssetsFsId(binding)} {
			if got, _, err := ValidatePluginUnixfsID(fsID, false); err != nil || got != id {
				t.Fatalf("filesystem binding: %q %v", got, err)
			}
		}
	}

	// Reject paths without a file or with a non-hash manifest segment.
	for _, path := range []string{"colors/manifest//entry.mjs", "colors/manifest/latest/entry.mjs", id} {
		if _, _, err := ParseHTTPPathPluginArtifact(path); err == nil {
			t.Errorf("invalid immutable path accepted: %s", path)
		}
	}
}

// TestPinAssetsHref pins the plugin's own asset paths to its manifest root.
func TestPinAssetsHref(t *testing.T) {
	// Build one context with a manifest root and one without.
	root, err := hash.Sum(hash.RecommendedHashType, []byte("module"))
	if err != nil {
		t.Fatal(err)
	}
	meta := &PluginMeta{PluginId: "colors"}
	pinned := NewPluginContextInfo(meta, "", root.MarshalString())
	unknown := NewPluginContextInfo(meta, "", "")
	href := PluginAssetsHttpPrefix + "colors/esb/viewer.js"

	// The pinned path selects the same file under the immutable artifact
	// ID.
	want := PluginAssetsHttpPrefix + PluginArtifactID("colors", root.MarshalString()) + "/esb/viewer.js"
	if got := pinned.PinAssetsHref(href); got != want {
		t.Fatalf("pinned href: %q, want %q", got, want)
	}

	// Leave other plugins' paths, and every path without a root, unchanged.
	other := PluginAssetsHttpPrefix + "shapes/esb/viewer.js"
	for _, tc := range []struct {
		info *PluginContextInfo
		href string
	}{{pinned, other}, {unknown, href}, {nil, href}} {
		if got := tc.info.PinAssetsHref(tc.href); got != tc.href {
			t.Errorf("unpinned href changed: %q to %q", tc.href, got)
		}
	}
}
