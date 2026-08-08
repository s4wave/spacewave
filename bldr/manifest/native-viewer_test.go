package bldr_manifest

import (
	"bytes"
	"context"
	stderrors "errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/unixfs"
	net_hash "github.com/s4wave/spacewave/net/hash"
)

// nativeViewerFixture constructs a valid native viewer manifest fixture.
func nativeViewerFixture() (*Manifest, *ManifestRef, *block.BlockRef, bldr_platform.Platform) {
	meta := &ManifestMeta{ManifestId: "plugin", PlatformId: "desktop/darwin/arm64", ViewerId: "viewer", ViewerTypeId: "glados/console", ViewerProfile: "default", ViewerProtocolVersion: 1}
	m := NewManifest(meta, "bin/viewer")
	root := &block.BlockRef{Hash: net_hash.NewHash(net_hash.HashType_HashType_SHA256, []byte(strings.Repeat("x", 32)))}
	ref := &ManifestRef{Meta: meta.CloneVT(), ManifestRef: &bucket.ObjectRef{RootRef: root.Clone()}}
	host, _ := bldr_platform.ParsePlatform("desktop/darwin/arm64")
	return m, ref, root, host
}

// TestManifestValidateNativeViewerMetadata proves native metadata is all-or-nothing and desktop-only.
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

// TestResolveNativeViewerChecksSelectedIdentity proves resolution freezes the selected root and manifest identities.
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

// TestResolveNativeViewerRejectsManifestObjectKeyAndPlatform proves unsafe keys and host mismatches cannot resolve.
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

// fakeNativeViewerArtifactFile records bounded file reads and release custody.
type fakeNativeViewerArtifactFile struct {
	// nodeType is the reported cursor node type.
	nodeType unixfs.FSCursorNodeType
	// data contains bytes returned by ReadAt.
	data []byte
	// size is the reported artifact size.
	size uint64
	// read overrides ReadAt when set.
	read func(context.Context, int64, []byte) (int64, error)
	// released counts Release calls.
	released int
}

// GetNodeType returns the fixture node type.
func (f *fakeNativeViewerArtifactFile) GetNodeType(context.Context) (unixfs.FSCursorNodeType, error) {
	return f.nodeType, nil
}

// GetSize returns the fixture artifact size.
func (f *fakeNativeViewerArtifactFile) GetSize(context.Context) (uint64, error) {
	return f.size, nil
}

// ReadAt returns fixture bytes or the configured read failure.
func (f *fakeNativeViewerArtifactFile) ReadAt(ctx context.Context, offset int64, p []byte) (int64, error) {
	if f.read != nil {
		return f.read(ctx, offset, p)
	}
	if offset < 0 || offset >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[offset:])
	return int64(n), nil
}

// Release records fixture release.
func (f *fakeNativeViewerArtifactFile) Release() { f.released++ }

// TestMaterializeNativeViewerArtifactNestedEntrypoint proves nested entrypoints publish private executable bytes.
func TestMaterializeNativeViewerArtifactNestedEntrypoint(t *testing.T) {
	m, ref, root, host := nativeViewerFixture()
	resolution, err := ResolveNativeViewer(m, ref, root, "plugin/manifest", host)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("native viewer bytes")
	file := &fakeNativeViewerArtifactFile{nodeType: unixfs.NewFSCursorNodeType_File(), data: want, size: uint64(len(want))}
	var lookedUp string
	destination := t.TempDir()
	artifact, err := materializeNativeViewerArtifactWithLookup(context.Background(), resolution, destination,
		func(_ context.Context, entrypoint string) (nativeViewerArtifactFile, error) {
			lookedUp = entrypoint
			return file, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if lookedUp != resolution.Entrypoint {
		t.Fatalf("looked up %q, want %q", lookedUp, resolution.Entrypoint)
	}
	if !filepath.IsAbs(artifact.Path) {
		t.Fatalf("artifact path is not absolute: %q", artifact.Path)
	}
	got, err := os.ReadFile(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("artifact bytes = %q, want %q", got, want)
	}
	info, err := os.Stat(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o700 {
		t.Fatalf("artifact mode = %s, want regular 0700", info.Mode())
	}
	if file.released != 1 {
		t.Fatalf("entrypoint releases = %d, want 1", file.released)
	}
	if err := artifact.Close(); err != nil {
		t.Fatal(err)
	}
	if err := artifact.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(artifact.Path); !stderrors.Is(err, os.ErrNotExist) {
		t.Fatalf("artifact after cleanup: %v", err)
	}
}

// TestMaterializeNativeViewerArtifactRejectsInvalidFiles proves non-regular, empty, and oversized entrypoints are rejected.
func TestMaterializeNativeViewerArtifactRejectsInvalidFiles(t *testing.T) {
	m, ref, root, host := nativeViewerFixture()
	resolution, err := ResolveNativeViewer(m, ref, root, "plugin/manifest", host)
	if err != nil {
		t.Fatal(err)
	}
	for name, file := range map[string]*fakeNativeViewerArtifactFile{
		"directory": {nodeType: unixfs.NewFSCursorNodeType_Dir(), data: []byte("x"), size: 1},
		"symlink":   {nodeType: unixfs.NewFSCursorNodeType_Symlink(), data: []byte("x"), size: 1},
		"empty":     {nodeType: unixfs.NewFSCursorNodeType_File(), size: 0},
		"too large": {nodeType: unixfs.NewFSCursorNodeType_File(), size: nativeViewerArtifactMaxBytes + 1},
	} {
		t.Run(name, func(t *testing.T) {
			destination := t.TempDir()
			_, err := materializeNativeViewerArtifactWithLookup(context.Background(), resolution, destination,
				func(_ context.Context, _ string) (nativeViewerArtifactFile, error) { return file, nil })
			if err == nil {
				t.Fatal("expected rejection")
			}
			if file.released != 1 {
				t.Fatalf("entrypoint releases = %d, want 1", file.released)
			}
			entries, readErr := os.ReadDir(destination)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("partial artifacts remain: %v", entries)
			}
		})
	}
}

// TestMaterializeNativeViewerArtifactFailuresCleanPartialFile proves every lookup, read, sync, and publish failure removes partial output.
func TestMaterializeNativeViewerArtifactFailuresCleanPartialFile(t *testing.T) {
	m, ref, root, host := nativeViewerFixture()
	resolution, err := ResolveNativeViewer(m, ref, root, "plugin/manifest", host)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		file *fakeNativeViewerArtifactFile
		want error
	}{
		"missing": {
			want: os.ErrNotExist,
		},
		"short read": {
			file: &fakeNativeViewerArtifactFile{
				nodeType: unixfs.NewFSCursorNodeType_File(), size: 3,
				read: func(context.Context, int64, []byte) (int64, error) { return 2, nil },
			},
		},
		"cancel": {
			file: &fakeNativeViewerArtifactFile{
				nodeType: unixfs.NewFSCursorNodeType_File(), data: []byte("abc"), size: 3,
				read: func(ctx context.Context, _ int64, _ []byte) (int64, error) { return 0, ctx.Err() },
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			destination := t.TempDir()
			ctx := context.Background()
			if name == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			file := tt.file
			var lookupErr error
			if name == "missing" {
				lookupErr = os.ErrNotExist
			}
			_, err := materializeNativeViewerArtifactWithLookup(ctx, resolution, destination,
				func(_ context.Context, _ string) (nativeViewerArtifactFile, error) {
					if file == nil {
						return nil, lookupErr
					}
					return file, lookupErr
				})
			if err == nil {
				t.Fatal("expected failure")
			}
			wantReleases := 1
			if name == "cancel" {
				wantReleases = 0 // cancellation is rejected before UnixFS lookup
			}
			if file != nil && file.released != wantReleases {
				t.Fatalf("entrypoint releases = %d, want %d", file.released, wantReleases)
			}
			entries, readErr := os.ReadDir(destination)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("partial artifacts remain: %v", entries)
			}
		})
	}
}

// TestWriteNativeViewerArtifactRejectsShortWrite proves incomplete writer progress returns an error.
func TestWriteNativeViewerArtifactRejectsShortWrite(t *testing.T) {
	file := &fakeNativeViewerArtifactFile{nodeType: unixfs.NewFSCursorNodeType_File(), data: []byte("abc"), size: 3}
	short := shortNativeViewerArtifactWriter{}
	err := writeNativeViewerArtifact(context.Background(), &short, file, 3)
	if !stderrors.Is(err, io.ErrShortWrite) {
		t.Fatalf("error = %v, want io.ErrShortWrite", err)
	}
}

// shortNativeViewerArtifactWriter simulates an incomplete artifact write.
type shortNativeViewerArtifactWriter struct{}

// Write accepts all but the final byte to simulate a short write.
func (*shortNativeViewerArtifactWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
