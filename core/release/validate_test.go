package release

import (
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

func TestReleaseGraphRoundTrip(t *testing.T) {
	ref := testBlockRef()
	manifestRef := &ManifestRef{Ref: ref}
	tests := []struct {
		name      string
		marshal   func() ([]byte, error)
		unmarshal func([]byte) error
		equal     func() bool
	}{
		{
			name:      "channel directory",
			marshal:   func() ([]byte, error) { return testChannelDirectory(ref).MarshalVT() },
			unmarshal: func(data []byte) error { return (&ChannelDirectory{}).UnmarshalVT(data) },
			equal: func() bool {
				msg := testChannelDirectory(ref)
				data, err := msg.MarshalVT()
				if err != nil {
					t.Fatalf("MarshalVT() error = %v", err)
				}
				got := &ChannelDirectory{}
				if err := got.UnmarshalVT(data); err != nil {
					t.Fatalf("UnmarshalVT() error = %v", err)
				}
				return msg.EqualVT(got)
			},
		},
		{
			name:      "release manifest",
			marshal:   func() ([]byte, error) { return testReleaseManifest(manifestRef).MarshalVT() },
			unmarshal: func(data []byte) error { return (&ReleaseManifest{}).UnmarshalVT(data) },
			equal: func() bool {
				msg := testReleaseManifest(manifestRef)
				data, err := msg.MarshalVT()
				if err != nil {
					t.Fatalf("MarshalVT() error = %v", err)
				}
				got := &ReleaseManifest{}
				if err := got.UnmarshalVT(data); err != nil {
					t.Fatalf("UnmarshalVT() error = %v", err)
				}
				return msg.EqualVT(got)
			},
		},
		{
			name:      "entrypoint manifest",
			marshal:   func() ([]byte, error) { return testEntrypointManifest(ref).MarshalVT() },
			unmarshal: func(data []byte) error { return (&EntrypointManifest{}).UnmarshalVT(data) },
			equal: func() bool {
				msg := testEntrypointManifest(ref)
				data, err := msg.MarshalVT()
				if err != nil {
					t.Fatalf("MarshalVT() error = %v", err)
				}
				got := &EntrypointManifest{}
				if err := got.UnmarshalVT(data); err != nil {
					t.Fatalf("UnmarshalVT() error = %v", err)
				}
				return msg.EqualVT(got)
			},
		},
		{
			name:      "plugin manifest",
			marshal:   func() ([]byte, error) { return testPluginManifest(ref).MarshalVT() },
			unmarshal: func(data []byte) error { return (&PluginManifest{}).UnmarshalVT(data) },
			equal: func() bool {
				msg := testPluginManifest(ref)
				data, err := msg.MarshalVT()
				if err != nil {
					t.Fatalf("MarshalVT() error = %v", err)
				}
				got := &PluginManifest{}
				if err := got.UnmarshalVT(data); err != nil {
					t.Fatalf("UnmarshalVT() error = %v", err)
				}
				return msg.EqualVT(got)
			},
		},
		{
			name:      "browser shell manifest",
			marshal:   func() ([]byte, error) { return testBrowserShellManifest(ref).MarshalVT() },
			unmarshal: func(data []byte) error { return (&BrowserShellManifest{}).UnmarshalVT(data) },
			equal: func() bool {
				msg := testBrowserShellManifest(ref)
				data, err := msg.MarshalVT()
				if err != nil {
					t.Fatalf("MarshalVT() error = %v", err)
				}
				got := &BrowserShellManifest{}
				if err := got.UnmarshalVT(data); err != nil {
					t.Fatalf("UnmarshalVT() error = %v", err)
				}
				return msg.EqualVT(got)
			},
		},
		{
			name:      "browser asset",
			marshal:   func() ([]byte, error) { return testBrowserAsset(ref).MarshalVT() },
			unmarshal: func(data []byte) error { return (&BrowserAsset{}).UnmarshalVT(data) },
			equal: func() bool {
				msg := testBrowserAsset(ref)
				data, err := msg.MarshalVT()
				if err != nil {
					t.Fatalf("MarshalVT() error = %v", err)
				}
				got := &BrowserAsset{}
				if err := got.UnmarshalVT(data); err != nil {
					t.Fatalf("UnmarshalVT() error = %v", err)
				}
				return msg.EqualVT(got)
			},
		},
		{
			name:      "update notification",
			marshal:   func() ([]byte, error) { return testUpdateNotification().MarshalVT() },
			unmarshal: func(data []byte) error { return (&UpdateNotification{}).UnmarshalVT(data) },
			equal: func() bool {
				msg := testUpdateNotification()
				data, err := msg.MarshalVT()
				if err != nil {
					t.Fatalf("MarshalVT() error = %v", err)
				}
				got := &UpdateNotification{}
				if err := got.UnmarshalVT(data); err != nil {
					t.Fatalf("UnmarshalVT() error = %v", err)
				}
				return msg.EqualVT(got)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.marshal()
			if err != nil {
				t.Fatalf("MarshalVT() error = %v", err)
			}
			if err := tt.unmarshal(data); err != nil {
				t.Fatalf("UnmarshalVT() error = %v", err)
			}
			if !tt.equal() {
				t.Fatalf("protobuf round trip changed message")
			}
		})
	}
}

func TestReleaseGraphValidation(t *testing.T) {
	ref := testBlockRef()
	manifestRef := &ManifestRef{Ref: ref}
	tests := []struct {
		name    string
		err     error
		wantErr string
	}{
		{
			name:    "channel directory nil ref",
			err:     (&ChannelDirectory{Channels: []*ChannelEntry{{ChannelKey: "stable"}}}).Validate(),
			wantErr: "invalid release manifest ref",
		},
		{
			name: "missing release manifest",
			err: testChannelDirectory(ref).ValidateReleaseManifestRefs(func(*block.BlockRef) bool {
				return false
			}),
			wantErr: "missing release manifest",
		},
		{
			name: "unknown platform key",
			err: (&ReleaseManifest{
				ProjectId:    "spacewave",
				Version:      "0.1.0",
				Entrypoints:  map[string]*ManifestRef{"darwin": manifestRef},
				BrowserShell: manifestRef,
			}).Validate(),
			wantErr: `unknown platform key "darwin"`,
		},
		{
			name:    "manifest ref nil ref",
			err:     (&ManifestRef{}).Validate(),
			wantErr: "nil block ref",
		},
		{
			name: "entrypoint nil ref",
			err: (&EntrypointManifest{
				Platform:    "darwin/arm64",
				Version:     "0.1.0",
				Size:        1,
				Sha256:      testSHA256(),
				ArchiveName: "spacewave-darwin-arm64.tar.gz",
			}).Validate(),
			wantErr: "invalid archive ref",
		},
		{
			name: "plugin nil manifest ref",
			err: (&PluginManifest{
				PluginId: "spacewave-web",
				Version:  "0.1.0",
			}).Validate(),
			wantErr: "invalid manifest ref",
		},
		{
			name: "browser asset nil ref",
			err: (&BrowserAsset{
				Path:        "/b/entrypoint/boot.mjs",
				Size:        1,
				Sha256:      testSHA256(),
				ContentType: "text/javascript",
			}).Validate(),
			wantErr: "invalid content ref",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(tt.err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, tt.err)
			}
		})
	}

	valid := []struct {
		name string
		err  error
	}{
		{name: "channel directory", err: testChannelDirectory(ref).Validate()},
		{name: "channel refs", err: testChannelDirectory(ref).ValidateReleaseManifestRefs(func(*block.BlockRef) bool { return true })},
		{name: "release manifest", err: testReleaseManifest(manifestRef).Validate()},
		{name: "manifest ref", err: manifestRef.Validate()},
		{name: "entrypoint manifest", err: testEntrypointManifest(ref).Validate()},
		{name: "plugin manifest", err: testPluginManifest(ref).Validate()},
		{name: "browser shell manifest", err: testBrowserShellManifest(ref).Validate()},
		{name: "browser asset", err: testBrowserAsset(ref).Validate()},
		{name: "update notification", err: testUpdateNotification().Validate()},
	}
	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err != nil {
				t.Fatalf("Validate() error = %v", tt.err)
			}
		})
	}
}

func testChannelDirectory(ref *block.BlockRef) *ChannelDirectory {
	return &ChannelDirectory{
		Channels: []*ChannelEntry{{
			ChannelKey:         "stable",
			ReleaseManifestRef: ref,
		}},
	}
}

func testReleaseManifest(ref *ManifestRef) *ReleaseManifest {
	return &ReleaseManifest{
		ProjectId: "spacewave",
		Rev:       1,
		Version:   "0.1.0",
		Entrypoints: map[string]*ManifestRef{
			"darwin/arm64": ref,
		},
		Plugins: map[string]*ManifestRef{
			"spacewave-web": ref,
		},
		BrowserShell:           ref,
		MinimumLauncherVersion: "0.1.0",
	}
}

func testEntrypointManifest(ref *block.BlockRef) *EntrypointManifest {
	return &EntrypointManifest{
		Platform:    "darwin/arm64",
		Version:     "0.1.0",
		ArchiveRef:  ref,
		Size:        1,
		Sha256:      testSHA256(),
		ArchiveName: "spacewave-darwin-arm64.tar.gz",
	}
}

func testPluginManifest(ref *block.BlockRef) *PluginManifest {
	return &PluginManifest{
		PluginId:    "spacewave-web",
		Version:     "0.1.0",
		ManifestRef: ref,
		ArtifactRef: ref,
	}
}

func testBrowserShellManifest(ref *block.BlockRef) *BrowserShellManifest {
	return &BrowserShellManifest{
		Version:           "0.1.0",
		GenerationId:      "gen-1",
		EntrypointPath:    "/b/entrypoint/boot.mjs",
		ServiceWorkerPath: "/b/entrypoint/sw.js",
		SharedWorkerPath:  "/b/entrypoint/shared-worker.js",
		WasmPath:          "/b/entrypoint/spacewave.wasm",
		Assets:            []*BrowserAsset{testBrowserAsset(ref)},
	}
}

func testBrowserAsset(ref *block.BlockRef) *BrowserAsset {
	return &BrowserAsset{
		Path:         "/b/entrypoint/boot.mjs",
		ContentRef:   ref,
		Size:         1,
		Sha256:       testSHA256(),
		ContentType:  "text/javascript",
		CacheControl: "public, max-age=31536000, immutable",
	}
}

func testUpdateNotification() *UpdateNotification {
	return &UpdateNotification{
		ChannelKey:     "stable",
		InnerSeqno:     1,
		RootPointerUrl: "https://example.invalid/root.packedmsg",
	}
}

func testBlockRef() *block.BlockRef {
	return &block.BlockRef{
		Hash: &hash.Hash{
			HashType: hash.HashType_HashType_SHA256,
			Hash:     testSHA256(),
		},
	}
}

func testSHA256() []byte {
	out := make([]byte, 32)
	out[0] = 1
	return out
}
