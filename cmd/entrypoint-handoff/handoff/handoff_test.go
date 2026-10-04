//go:build !js

package handoff

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/testbed"
	db_world "github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestNeedsBuilderImage(t *testing.T) {
	tests := []struct {
		name      string
		hostGOOS  string
		platforms []string
		want      bool
	}{
		{
			name:      "darwin only never needs docker builder",
			hostGOOS:  "darwin",
			platforms: []string{"darwin-amd64", "darwin-arm64"},
			want:      false,
		},
		{
			name:      "linux needs docker builder",
			hostGOOS:  "linux",
			platforms: []string{"linux-amd64"},
			want:      true,
		},
		{
			name:      "windows on linux needs docker builder",
			hostGOOS:  "linux",
			platforms: []string{"windows-amd64"},
			want:      true,
		},
		{
			name:      "windows on windows builds natively",
			hostGOOS:  "windows",
			platforms: []string{"windows-amd64", "windows-arm64"},
			want:      false,
		},
		{
			name:      "windows and linux on windows still need docker builder for linux",
			hostGOOS:  "windows",
			platforms: []string{"windows-amd64", "linux-amd64"},
			want:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := needsBuilderImage(test.hostGOOS, test.platforms)
			if got != test.want {
				t.Fatalf("needsBuilderImage(%q, %#v) = %v, want %v", test.hostGOOS, test.platforms, got, test.want)
			}
		})
	}
}

func TestWriteEntrypointHandoffManifestRecordsStringRevisionAndFiles(t *testing.T) {
	// Create browser release files in a handoff root.
	root := t.TempDir()
	browserRelease := filepath.Join(root, "browser-staging", "app", "browser-release.json")
	staticIndex := filepath.Join(root, "browser-staging", "static", "index.html")
	writeTestFile(t, browserRelease)
	writeTestFile(t, staticIndex)

	// Write an entrypoint handoff with a string revision and source provenance.
	if err := WriteEntrypointHandoffManifest(EntrypointHandoffOptions{
		RootDir:            root,
		BrowserStagingDir:  filepath.Join(root, "browser-staging"),
		Version:            "0.51.7",
		Rev:                "31",
		GitSHA:             "abc123",
		Tag:                "v0.51.7",
		ReleaseEnvironment: "production",
		RunID:              "123",
		RunAttempt:         "2",
		SourceRepo:         "s4wave/spacewave",
		Workflow:           "entrypoint-release",
	}); err != nil {
		t.Fatalf("WriteEntrypointHandoffManifest() error = %v", err)
	}

	// Read and parse the written entrypoint handoff manifest.
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	// Verify the manifest preserves the string revision and source run.
	if got := string(v.GetStringBytes("rev")); got != "31" {
		t.Fatalf("rev = %q, want 31", got)
	}
	if got := string(v.GetStringBytes("run_id")); got != "123" {
		t.Fatalf("run_id = %q, want 123", got)
	}

	// Verify the browser entries use handoff-relative paths.
	entries := v.GetArray("browser_staging")
	if len(entries) != 2 {
		t.Fatalf("browser_staging entries = %d, want 2", len(entries))
	}
	if got := string(entries[0].GetStringBytes("path")); got != "browser-staging/app/browser-release.json" {
		t.Fatalf("first browser path = %q", got)
	}
}

func TestWriteEntrypointHandoffManifestRejectsSymlink(t *testing.T) {
	// Create a handoff whose browser staging tree holds a symlink.
	root := t.TempDir()
	browserRelease := filepath.Join(root, "browser-staging", "app", "browser-release.json")
	link := filepath.Join(root, "browser-staging", "static", "index.html")
	writeTestFile(t, browserRelease)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(browserRelease, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	// Attempt to write the entrypoint handoff containing the symlink.
	err := WriteEntrypointHandoffManifest(EntrypointHandoffOptions{
		RootDir:            root,
		BrowserStagingDir:  filepath.Join(root, "browser-staging"),
		Version:            "0.51.7",
		Rev:                "31",
		GitSHA:             "abc123",
		Tag:                "v0.51.7",
		ReleaseEnvironment: "production",
		RunID:              "123",
		RunAttempt:         "2",
		SourceRepo:         "s4wave/spacewave",
		Workflow:           "entrypoint-release",
	})

	// Verify the handoff writer rejects the symlink.
	if err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestStageCLIHandoffArtifactsUsesNativeCLIRoot(t *testing.T) {
	// Create a CLI archive and an empty handoff root.
	root := t.TempDir()
	artifactsDir := filepath.Join(t.TempDir(), "cli")
	writeTestFileBytes(t, filepath.Join(artifactsDir, "spacewave-cli-linux-amd64.tar.gz"), []byte("cli archive"))

	// Stage the CLI archive under the handoff root.
	artifacts, err := stageCLIHandoffArtifacts(root, artifactsDir)
	if err != nil {
		t.Fatalf("stageCLIHandoffArtifacts() error = %v", err)
	}

	// Verify the CLI entry path is relative to the native root.
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %d, want 1", len(artifacts))
	}
	if got := artifacts[0].Path; got != "cli/spacewave-cli-linux-amd64.tar.gz" {
		t.Fatalf("artifact path = %q", got)
	}

	// Read the staged archive from the native CLI directory.
	staged := filepath.Join(root, "native", "cli", "spacewave-cli-linux-amd64.tar.gz")
	data, err := os.ReadFile(staged)
	if err != nil {
		t.Fatalf("read staged artifact: %v", err)
	}

	// Verify staging preserves the CLI archive contents.
	if string(data) != "cli archive" {
		t.Fatalf("staged artifact data = %q", string(data))
	}
}

func TestCLIBinaryPathUsesCLIManifestBuildRoot(t *testing.T) {
	got := cliEntrypointBinaryPath("/repo", "linux", "amd64", "spacewave")
	want := filepath.Join("/repo", ".bldr", "build", "desktop", "linux", "amd64", "spacewave-cli", "dist", "spacewave")
	if got != want {
		t.Fatalf("cli binary path = %q, want %q", got, want)
	}
}

func TestCollectCLIHandoffManifestRefsAcceptsReleaseBuildTypeAndRejectsDev(t *testing.T) {
	// Create a World with release manifests for every CLI platform.
	ctx := context.Background()
	releaseWorld := newTestCLIHandoffWorld(t, ctx, func(string) string {
		return string(bldr_manifest.BuildType_RELEASE)
	})

	// Collect the release manifest references from the World.
	refs, err := collectCLIHandoffManifestRefs(ctx, releaseWorld)
	if err != nil {
		t.Fatalf("collectCLIHandoffManifestRefs(release) error = %v", err)
	}

	// Verify the release references cover the CLI platforms in order.
	if len(refs) != len(cliHandoffPlatformIDs) {
		t.Fatalf("release manifest refs = %d, want %d", len(refs), len(cliHandoffPlatformIDs))
	}
	for idx, platformID := range cliHandoffPlatformIDs {
		ref := refs[idx]
		if ref.ManifestID != cliEntrypointManifestID {
			t.Fatalf("release ref[%d] manifest id = %q, want %q", idx, ref.ManifestID, cliEntrypointManifestID)
		}
		if ref.PlatformID != platformID {
			t.Fatalf("release ref[%d] platform id = %q, want %q", idx, ref.PlatformID, platformID)
		}
	}

	// Create a World with a development manifest among the CLI releases.
	devWorld := newTestCLIHandoffWorld(t, ctx, func(platformID string) string {
		if platformID == "desktop/linux/amd64" {
			return string(bldr_manifest.BuildType_DEV)
		}
		return string(bldr_manifest.BuildType_RELEASE)
	})

	// Collect the CLI references from the mixed-build World.
	_, err = collectCLIHandoffManifestRefs(ctx, devWorld)

	// Verify the development manifest prevents a release handoff.
	if err == nil || !strings.Contains(err.Error(), "wrong build type: dev") {
		t.Fatalf("collectCLIHandoffManifestRefs(dev) error = %v, want wrong build type rejection", err)
	}
}

func TestStageStaticHTMLCopiesSiteFiles(t *testing.T) {
	// Create separate prerender and browser staging directories.
	prerenderDir := t.TempDir()
	stagingDir := t.TempDir()

	// Populate prerender output with supported site files and unrelated notes.
	for _, name := range []string{"sitemap.xml", "llms.txt", "notes.md"} {
		if err := os.WriteFile(filepath.Join(prerenderDir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Stage the static site files into the browser output.
	if err := stageStaticHTML(prerenderDir, stagingDir); err != nil {
		t.Fatalf("stage static HTML: %v", err)
	}

	// Verify the supported site files reached the static staging directory.
	for _, name := range []string{"sitemap.xml", "llms.txt"} {
		if _, err := os.Stat(filepath.Join(stagingDir, "static", name)); err != nil {
			t.Fatalf("expected staged %s: %v", name, err)
		}
	}

	// Verify unrelated notes are excluded from static staging.
	if _, err := os.Stat(filepath.Join(stagingDir, "static", "notes.md")); !os.IsNotExist(err) {
		t.Fatalf("expected notes.md to be skipped, got err=%v", err)
	}
}

func TestValidatePlatformBundleInputsAcceptsCompleteMatrix(t *testing.T) {
	// Create desktop, CLI, and helper binaries for each bundle platform.
	dir := t.TempDir()
	platforms := []string{"darwin-arm64", "linux-amd64", "windows-arm64"}
	for _, platform := range platforms {
		// Choose the executable names for this platform.
		goos, _ := splitPlatform(platform)
		binName := "spacewave"
		helperName := "spacewave-helper"
		if goos == "windows" {
			binName += ".exe"
			helperName += ".exe"
		}

		// Write the desktop, CLI, and helper inputs for this platform.
		writeTestFile(t, filepath.Join(dir, ".tmp", "dist", platform, binName))
		writeTestFile(t, filepath.Join(dir, ".tmp", "dist-cli", platform, binName))
		writeTestFile(t, filepath.Join(dir, "dist", "helper", platform, helperName))
	}

	// Supply the icons and desktop launcher used by the platform bundles.
	for _, iconName := range []string{
		"icon.icns",
		"icon.ico",
		"icon-48.png",
		"icon-128.png",
		"icon-256.png",
	} {
		writeTestFile(t, filepath.Join(dir, ".tmp", "icons", iconName))
	}
	writeTestFile(t, filepath.Join(dir, ".tmp", "spacewave.desktop"))

	// Verify the complete platform input matrix passes validation.
	if err := validatePlatformBundleInputs(dir, platforms); err != nil {
		t.Fatalf("validatePlatformBundleInputs complete matrix = %v", err)
	}
}

func TestValidatePlatformBundleInputsRejectsMissingHelper(t *testing.T) {
	// Create Linux bundle inputs with the helper binary absent.
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".tmp", "dist", "linux-amd64", "spacewave"))
	writeTestFile(t, filepath.Join(dir, ".tmp", "dist-cli", "linux-amd64", "spacewave"))
	writeTestFile(t, filepath.Join(dir, ".tmp", "icons", "icon-256.png"))
	writeTestFile(t, filepath.Join(dir, ".tmp", "spacewave.desktop"))

	// Verify platform validation rejects the missing helper binary.
	if err := validatePlatformBundleInputs(dir, []string{"linux-amd64"}); err == nil {
		t.Fatal("validatePlatformBundleInputs accepted missing helper")
	}
}

func TestValidatePackagedArtifactsAcceptsCompleteMatrix(t *testing.T) {
	dir := t.TempDir()
	platforms := []string{"darwin-arm64", "linux-amd64", "windows-arm64"}
	for _, platform := range platforms {
		goos, goarch := splitPlatform(platform)
		writeTestFile(t, filepath.Join(dir, ".tmp", "dist", "bundles", archiveName(goos, platform)))
		writeTestFile(t, filepath.Join(dir, "dist", "cli", cliArchiveName(goos, platform)))
		switch goos {
		case "darwin":
			writeTestFile(t, filepath.Join(dir, "dist", "installers", "spacewave-macos-"+goarch+".dmg"))
		case "linux":
			writeTestFile(t, filepath.Join(dir, "dist", "installers", "spacewave-linux-"+goarch+".AppImage"))
		case "windows":
			writeTestFile(t, filepath.Join(dir, "dist", "installers", "spacewave-windows-"+goarch+".msix"))
			writeTestFile(t, filepath.Join(dir, "dist", "installers", "spacewave-windows-"+goarch+".zip"))
		}
	}

	if err := validatePackagedArtifacts(dir, platforms); err != nil {
		t.Fatalf("validatePackagedArtifacts complete matrix = %v", err)
	}
}

func TestValidateBrowserBundleArtifactsChecksBrowserOutputs(t *testing.T) {
	// Create the browser release metadata and index.
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "staging", "app", "browser-release.json"))
	writeTestFile(t, filepath.Join(dir, "staging", "static", "index.html"))

	// Verify the complete browser output passes validation.
	if err := validateBrowserBundleArtifacts(dir); err != nil {
		t.Fatalf("validateBrowserBundleArtifacts complete outputs = %v", err)
	}

	// Remove the staged browser index from the release output.
	if err := os.Remove(filepath.Join(dir, "staging", "static", "index.html")); err != nil {
		t.Fatal(err)
	}

	// Verify browser validation rejects the missing index.
	if err := validateBrowserBundleArtifacts(dir); err == nil {
		t.Fatal("validateBrowserBundleArtifacts accepted missing index html")
	}
}

func newTestCLIHandoffWorld(
	t *testing.T,
	ctx context.Context,
	buildTypeForPlatform func(string) string,
) db_world.WorldState {
	// Attribute CLI World setup failures to the calling test.
	t.Helper()

	// Start a storage testbed for the CLI manifest World.
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Open an empty cursor for constructing the CLI manifest World.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(ocs.Release)

	// Create the World and its devtool manifest store.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, "devtool"); err != nil {
		t.Fatal(err.Error())
	}

	// Store a CLI manifest for each supported handoff platform.
	for idx, platformID := range cliHandoffPlatformIDs {
		manifestRef := newTestCLIHandoffManifestRef(
			t,
			ctx,
			tb,
			buildTypeForPlatform(platformID),
			platformID,
			uint64(idx+1),
		)
		objectKey := "devtool/manifests/" + platformID
		if err := bldr_manifest_world.ExStoreManifestOp(
			ctx,
			ws,
			peer.ID("test"),
			objectKey,
			[]string{"devtool"},
			manifestRef,
		); err != nil {
			t.Fatal(err.Error())
		}
	}
	return ws
}

func newTestCLIHandoffManifestRef(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	buildType string,
	platformID string,
	rev uint64,
) *bldr_manifest.ManifestRef {
	// Attribute CLI manifest setup failures to the calling test.
	t.Helper()

	// Describe the CLI manifest release and target platform.
	meta := &bldr_manifest.ManifestMeta{
		ManifestId: cliEntrypointManifestID,
		BuildType:  buildType,
		PlatformId: platformID,
		Rev:        rev,
	}

	// Open an empty cursor for writing the CLI manifest block.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer oc.Release()

	// Persist the CLI manifest block and publish its root reference.
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(bldr_manifest.NewManifest(meta, "entrypoint"), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc.SetRootRef(rootRef)
	return bldr_manifest.NewManifestRef(meta, oc.GetRef())
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	writeTestFileBytes(t, path, []byte("artifact"))
}

func writeTestFileBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatal(err)
	}
}
