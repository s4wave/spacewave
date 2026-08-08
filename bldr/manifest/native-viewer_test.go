package bldr_manifest

import (
	"strings"
	"testing"

	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	net_hash "github.com/s4wave/spacewave/net/hash"
)

func nativeViewerFixture() (*Manifest, *ManifestRef, *block.BlockRef, bldr_platform.Platform) {
	meta := &ManifestMeta{ManifestId: "plugin", PlatformId: "desktop/darwin/arm64", ViewerId: "viewer", ViewerTypeId: "glados/console", ViewerProfile: "default", ViewerProtocolVersion: 1}
	m := NewManifest(meta, "bin/viewer")
	root := &block.BlockRef{Hash: net_hash.NewHash(net_hash.HashType_HashType_SHA256, []byte(strings.Repeat("x", 32)))}
	ref := &ManifestRef{Meta: meta.CloneVT(), ManifestRef: &bucket.ObjectRef{RootRef: root.Clone()}}
	host, _ := bldr_platform.ParsePlatform("desktop/darwin/arm64")
	return m, ref, root, host
}

func TestManifestValidateNativeViewerMetadata(t *testing.T) {
	m, _, _, _ := nativeViewerFixture()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	m.Meta.ViewerProfile = "bad/profile"
	if err := m.Validate(); err == nil {
		t.Fatal("expected unsafe profile rejection")
	}
}

func TestResolveNativeViewerChecksSelectedIdentity(t *testing.T) {
	m, ref, root, host := nativeViewerFixture()
	got, err := ResolveNativeViewer(m, ref, root, "plugin/manifest", host)
	if err != nil {
		t.Fatal(err)
	}
	if got.PluginID != "plugin" || got.ManifestObjectKey != "plugin/manifest" || got.ManifestDigest == "" || got.ViewerID != "viewer" || got.ViewerTypeID != "glados/console" || got.ViewerProfile != "default" || got.ProtocolVersion != 1 || got.Entrypoint != "bin/viewer" || got.PlatformID != "desktop/darwin/arm64" {
		t.Fatalf("unexpected resolution: %#v", got)
	}
	bad := root.Clone()
	bad.Hash.Hash[0]++
	if _, err := ResolveNativeViewer(m, ref, bad, "plugin/manifest", host); err == nil {
		t.Fatal("expected root mismatch")
	}
	ref.Meta.ViewerProfile = "other"
	if _, err := ResolveNativeViewer(m, ref, root, "plugin/manifest", host); err == nil {
		t.Fatal("expected metadata mismatch")
	}
}

func TestResolveNativeViewerRejectsManifestObjectKeyAndPlatform(t *testing.T) {
	m, ref, root, host := nativeViewerFixture()
	for _, key := range []string{"", "bad\x00key", strings.Repeat("x", 1001)} {
		if _, err := ResolveNativeViewer(m, ref, root, key, host); err == nil {
			t.Errorf("key %q accepted", key)
		}
	}
	other, _ := bldr_platform.ParsePlatform("desktop/linux/amd64")
	if _, err := ResolveNativeViewer(m, ref, root, "plugin/manifest", other); err == nil {
		t.Fatal("expected host mismatch")
	}
	m.Meta.ViewerProfile = ""
	if err := m.Validate(); err == nil {
		t.Fatal("expected partial metadata rejection")
	}
	m.Meta.ViewerProfile = "default"
	m.Meta.PlatformId = "web/js/wasm"
	if err := m.Validate(); err == nil {
		t.Fatal("expected non-native rejection")
	}
	m.Meta.PlatformId = "desktop/darwin/arm64"
	m.Entrypoint = "../viewer"
	if err := m.Validate(); err == nil {
		t.Fatal("expected unsafe entrypoint rejection")
	}
}
