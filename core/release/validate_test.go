package release

import (
	"strings"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/hash"
)

func TestReleaseMetadataRoundTrip(t *testing.T) {
	ref := testBlockRef()
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
			name:      "release metadata",
			marshal:   func() ([]byte, error) { return testReleaseMetadata(ref).MarshalVT() },
			unmarshal: func(data []byte) error { return (&ReleaseMetadata{}).UnmarshalVT(data) },
			equal: func() bool {
				msg := testReleaseMetadata(ref)
				data, err := msg.MarshalVT()
				if err != nil {
					t.Fatalf("MarshalVT() error = %v", err)
				}
				got := &ReleaseMetadata{}
				if err := got.UnmarshalVT(data); err != nil {
					t.Fatalf("UnmarshalVT() error = %v", err)
				}
				return msg.EqualVT(got)
			},
		},
		{
			name:      "browser shell metadata",
			marshal:   func() ([]byte, error) { return testBrowserShellMetadata(ref).MarshalVT() },
			unmarshal: func(data []byte) error { return (&BrowserShellMetadata{}).UnmarshalVT(data) },
			equal: func() bool {
				msg := testBrowserShellMetadata(ref)
				data, err := msg.MarshalVT()
				if err != nil {
					t.Fatalf("MarshalVT() error = %v", err)
				}
				got := &BrowserShellMetadata{}
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

func TestReleaseMetadataValidation(t *testing.T) {
	ref := testBlockRef()
	tests := []struct {
		name    string
		err     error
		wantErr string
	}{
		{
			name:    "channel directory nil ref",
			err:     (&ChannelDirectory{Channels: []*ChannelEntry{{ChannelKey: "stable"}}}).Validate(),
			wantErr: "invalid release metadata ref",
		},
		{
			name: "missing release metadata",
			err: testChannelDirectory(ref).ValidateReleaseMetadataRefs(func(*block.BlockRef) bool {
				return false
			}),
			wantErr: "missing release metadata",
		},
		{
			name: "missing bldr manifest refs",
			err: (&ReleaseMetadata{
				ProjectId:    "spacewave",
				ChannelKey:   "stable",
				Version:      "0.1.0",
				BrowserShell: testBrowserShellMetadata(ref),
			}).Validate(),
			wantErr: "no bldr manifest refs",
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
		{name: "channel refs", err: testChannelDirectory(ref).ValidateReleaseMetadataRefs(func(*block.BlockRef) bool { return true })},
		{name: "release metadata", err: testReleaseMetadata(ref).Validate()},
		{name: "browser shell metadata", err: testBrowserShellMetadata(ref).Validate()},
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
			ReleaseMetadataRef: ref,
		}},
	}
}

func testReleaseMetadata(ref *block.BlockRef) *ReleaseMetadata {
	return &ReleaseMetadata{
		ProjectId:    "spacewave",
		Rev:          1,
		Version:      "0.1.0",
		ChannelKey:   "stable",
		ManifestRefs: []*bldr_manifest.ManifestRef{testManifestRef(ref)},
		BrowserShell:           testBrowserShellMetadata(ref),
		MinimumLauncherVersion: "0.1.0",
	}
}

func testManifestRef(ref *block.BlockRef) *bldr_manifest.ManifestRef {
	return &bldr_manifest.ManifestRef{
		Meta: &bldr_manifest.ManifestMeta{
			ManifestId: "spacewave-web",
			BuildType:  "production",
			PlatformId: "js",
			Rev:        1,
		},
		ManifestRef: &bucket.ObjectRef{RootRef: ref},
	}
}

func testBrowserShellMetadata(ref *block.BlockRef) *BrowserShellMetadata {
	return &BrowserShellMetadata{
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
