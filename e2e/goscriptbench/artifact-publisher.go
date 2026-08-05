//go:build !js

package goscriptbench

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"

	"github.com/pkg/errors"
)

const (
	artifactManifestFile          = "manifest.json"
	artifactResultFile            = "result.json"
	artifactDiagnosticFile        = "diagnostic.json"
	artifactRuntimeTraceFile      = "runtime.trace"
	artifactBrowserCPUProfileFile = "browser.cpuprofile"
)

// ArtifactPublisher atomically exposes complete per-engine artifact directories.
type ArtifactPublisher struct {
	// root is the absolute artifact root
	root string
}

// NewArtifactPublisher constructs a publisher rooted at outputRoot.
func NewArtifactPublisher(outputRoot string) (*ArtifactPublisher, error) {
	if outputRoot == "" {
		return nil, errors.New("artifact output root is required")
	}
	root, err := filepath.Abs(outputRoot)
	if err != nil {
		return nil, errors.Wrap(err, "resolve artifact output root")
	}
	return &ArtifactPublisher{root: root}, nil
}

// Publish validates and atomically exposes one per-engine artifact bundle.
func (p *ArtifactPublisher) Publish(bundle ArtifactBundle) (string, error) {
	if p == nil || p.root == "" {
		return "", errors.New("artifact publisher is not initialized")
	}
	if err := bundle.Validate(); err != nil {
		return "", err
	}

	// Encode every file and its integrity manifest before touching the destination.
	resultData := marshalArtifactData(bundle.Result)
	diagnosticData := marshalDiagnosticData(bundle.Diagnostic)
	manifest := artifactManifest{
		SchemaVersion:      artifactSchemaVersion,
		ResultFile:         artifactResultFile,
		ResultSHA256:       artifactDigest(resultData),
		DiagnosticFile:     artifactDiagnosticFile,
		DiagnosticSHA256:   artifactDigest(diagnosticData),
		RuntimeTraceFile:   artifactRuntimeTraceFile,
		RuntimeTraceSHA256: artifactDigest(bundle.RuntimeTrace),
	}
	files := []struct {
		name string
		data []byte
	}{
		{name: artifactResultFile, data: resultData},
		{name: artifactDiagnosticFile, data: diagnosticData},
		{name: artifactRuntimeTraceFile, data: bundle.RuntimeTrace},
	}
	if len(bundle.BrowserCPUProfile) != 0 {
		manifest.BrowserCPUProfileFile = artifactBrowserCPUProfileFile
		manifest.BrowserCPUProfileSHA256 = artifactDigest(bundle.BrowserCPUProfile)
		files = append(files, struct {
			name string
			data []byte
		}{name: artifactBrowserCPUProfileFile, data: bundle.BrowserCPUProfile})
	}
	manifestData := marshalManifestData(manifest)
	files = append(files, struct {
		name string
		data []byte
	}{name: artifactManifestFile, data: manifestData})

	// Reserve a hidden sibling directory so only the final rename is observable.
	runDir := filepath.Join(p.root, bundle.Result.Metadata.RunID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", errors.Wrap(err, "create artifact run directory")
	}
	finalDir := filepath.Join(runDir, bundle.Result.Metadata.Engine)
	_, statErr := os.Lstat(finalDir)
	if statErr == nil {
		return "", errors.Errorf("artifact destination already exists: %s", finalDir)
	}
	if !os.IsNotExist(statErr) {
		return "", errors.Wrap(statErr, "inspect artifact destination")
	}
	tempDir, err := os.MkdirTemp(runDir, "."+bundle.Result.Metadata.Engine+"-")
	if err != nil {
		return "", errors.Wrap(err, "create temporary artifact directory")
	}
	defer func() {
		if tempDir != "" {
			// A failed temporary cleanup cannot make an unpublished artifact visible.
			_ = os.RemoveAll(tempDir)
		}
	}()

	// Write the complete bundle under the hidden name, then publish it in one rename.
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(tempDir, file.name), file.data, 0o644); err != nil {
			return "", errors.Wrapf(err, "write artifact file %s", file.name)
		}
	}
	if err := os.Rename(tempDir, finalDir); err != nil {
		return "", errors.Wrap(err, "publish artifact directory")
	}
	tempDir = ""
	return finalDir, nil
}

// ReadArtifact verifies and decodes one published per-engine artifact directory.
func ReadArtifact(dir string) (*ArtifactBundle, error) {
	manifestData, err := os.ReadFile(filepath.Join(dir, artifactManifestFile))
	if err != nil {
		return nil, errors.Wrap(err, "read artifact manifest")
	}
	manifest, err := parseManifestData(manifestData)
	if err != nil {
		return nil, err
	}
	if manifest.SchemaVersion != artifactSchemaVersion {
		return nil, errors.Errorf("artifact manifest schema version %d is unsupported", manifest.SchemaVersion)
	}
	if manifest.ResultFile != artifactResultFile ||
		manifest.DiagnosticFile != artifactDiagnosticFile ||
		manifest.RuntimeTraceFile != artifactRuntimeTraceFile {
		return nil, errors.New("artifact manifest names unexpected files")
	}
	if manifest.BrowserCPUProfileFile != "" &&
		manifest.BrowserCPUProfileFile != artifactBrowserCPUProfileFile {
		return nil, errors.New("artifact manifest names an unexpected browser CPU profile")
	}

	// Require every published file to be named by the artifact contract.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrap(err, "read artifact directory")
	}
	files := make([]string, len(entries))
	for idx, entry := range entries {
		if entry.IsDir() {
			return nil, errors.Errorf("artifact contains unexpected directory %q", entry.Name())
		}
		files[idx] = entry.Name()
	}
	expectedFiles := []string{
		artifactDiagnosticFile,
		artifactManifestFile,
		artifactResultFile,
		artifactRuntimeTraceFile,
	}
	if manifest.BrowserCPUProfileFile != "" {
		expectedFiles = append(expectedFiles, artifactBrowserCPUProfileFile)
	}
	slices.Sort(files)
	slices.Sort(expectedFiles)
	if !slices.Equal(files, expectedFiles) {
		return nil, errors.Errorf("artifact files = %q, expected %q", files, expectedFiles)
	}

	// Verify file identities before accepting their semantic content.
	resultData, err := os.ReadFile(filepath.Join(dir, manifest.ResultFile))
	if err != nil {
		return nil, errors.Wrap(err, "read result artifact")
	}
	if artifactDigest(resultData) != manifest.ResultSHA256 {
		return nil, errors.New("result artifact digest differs from its manifest")
	}
	diagnosticData, err := os.ReadFile(filepath.Join(dir, manifest.DiagnosticFile))
	if err != nil {
		return nil, errors.Wrap(err, "read diagnostic artifact")
	}
	if artifactDigest(diagnosticData) != manifest.DiagnosticSHA256 {
		return nil, errors.New("diagnostic artifact digest differs from its manifest")
	}

	// Verify diagnostic evidence before accepting its semantic content.
	runtimeTrace, err := os.ReadFile(filepath.Join(dir, manifest.RuntimeTraceFile))
	if err != nil {
		return nil, errors.Wrap(err, "read runtime trace")
	}
	if artifactDigest(runtimeTrace) != manifest.RuntimeTraceSHA256 {
		return nil, errors.New("runtime trace digest differs from its manifest")
	}
	var browserCPUProfile []byte
	if (manifest.BrowserCPUProfileFile == "") != (manifest.BrowserCPUProfileSHA256 == "") {
		return nil, errors.New("browser CPU profile manifest identity is incomplete")
	}
	if manifest.BrowserCPUProfileFile != "" {
		browserCPUProfile, err = os.ReadFile(filepath.Join(dir, manifest.BrowserCPUProfileFile))
		if err != nil {
			return nil, errors.Wrap(err, "read browser CPU profile")
		}
		if artifactDigest(browserCPUProfile) != manifest.BrowserCPUProfileSHA256 {
			return nil, errors.New("browser CPU profile digest differs from its manifest")
		}
	}

	// Decode and validate every cross-file invariant before returning the bundle.
	result, err := parseArtifactData(resultData)
	if err != nil {
		return nil, err
	}
	diagnostic, err := parseDiagnosticData(diagnosticData)
	if err != nil {
		return nil, err
	}
	bundle := &ArtifactBundle{
		Result:            result,
		Diagnostic:        diagnostic,
		RuntimeTrace:      runtimeTrace,
		BrowserCPUProfile: browserCPUProfile,
	}
	if err := bundle.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate decoded artifact")
	}
	return bundle, nil
}

func artifactDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
