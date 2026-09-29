//go:build !js

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	bldr_manifest_pack "github.com/s4wave/spacewave/bldr/manifest/pack"
)

type pluginHandoffOptions struct {
	rootDir            string
	manifestRefsPath   string
	pluginRev          string
	releaseEnvironment string
	requestedSelection string
	gitSHA             string
	runID              string
	runAttempt         string
	sourceRepo         string
	workflow           string
	includeBrowser     bool
	includeMacOS       bool
	includeWindows     bool
	includeLinux       bool
}

// pluginHandoffEntry describes one file of the handoff root.
type pluginHandoffEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func runWritePluginHandoffManifest(args []string) error {
	// Declare the write-handoff-manifest flags.
	fs := flag.NewFlagSet("write-handoff-manifest", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts pluginHandoffOptions
	if err := func() error {
		fs.StringVar(&opts.rootDir, "root", "", "path to the plugin handoff root")
		fs.StringVar(&opts.manifestRefsPath, "manifest-refs", "", "path to manifest refs JSON")
		fs.StringVar(&opts.pluginRev, "plugin-rev", "", "plugin release revision")
		fs.StringVar(&opts.releaseEnvironment, "release-environment", "", "release environment")
		fs.StringVar(&opts.requestedSelection, "requested-selection", "everything", "requested plugin selection")
		fs.StringVar(&opts.gitSHA, "git-sha", "", "source git SHA")
		fs.StringVar(&opts.runID, "run-id", "", "source GitHub run id")
		fs.StringVar(&opts.runAttempt, "run-attempt", "", "source GitHub run attempt")
		fs.StringVar(&opts.sourceRepo, "source-repo", "", "source GitHub repository")
		fs.StringVar(&opts.workflow, "workflow", "", "source GitHub workflow")
		fs.BoolVar(&opts.includeBrowser, "include-browser", false, "browser surface was produced")
		fs.BoolVar(&opts.includeMacOS, "include-macos", false, "macOS surface was produced")
		fs.BoolVar(&opts.includeWindows, "include-windows", false, "Windows surface was produced")
		fs.BoolVar(&opts.includeLinux, "include-linux", false, "Linux surface was produced")
		return fs.Parse(args)
	}(); err != nil {
		return errors.Wrap(err, "parse flags")
	}

	// Write the manifest.
	return writePluginHandoffManifest(opts)
}

// writePluginHandoffManifest describes the pack files in the handoff root and
// writes manifest.json beside them.
func writePluginHandoffManifest(opts pluginHandoffOptions) error {
	// Require the identity and provenance every consumer checks.
	if opts.rootDir == "" {
		return errors.New("handoff root dir is required")
	}
	if opts.manifestRefsPath == "" {
		return errors.New("manifest refs path is required")
	}
	if opts.pluginRev == "" {
		return errors.New("plugin rev is required")
	}
	if opts.releaseEnvironment == "" {
		return errors.New("release environment is required")
	}
	if opts.gitSHA == "" || opts.runID == "" || opts.runAttempt == "" || opts.sourceRepo == "" || opts.workflow == "" {
		return errors.New("source provenance is required")
	}

	// Read the encoded manifest references.
	var parser fastjson.Parser
	manifestRefs, err := readPluginManifestRefs(&parser, opts.manifestRefsPath)
	if err != nil {
		return err
	}

	// Describe the pack and its metadata.
	pack, err := buildPluginEntry(opts.rootDir, filepath.Join(opts.rootDir, bldr_manifest_pack.ArtifactPackFilename))
	if err != nil {
		return errors.Wrap(err, "collect manifest pack")
	}
	packMeta, err := buildPluginEntry(opts.rootDir, filepath.Join(opts.rootDir, bldr_manifest_pack.ArtifactMetadataFilename))
	if err != nil {
		return errors.Wrap(err, "collect manifest pack metadata")
	}

	// Write manifest.json.
	data := marshalPluginHandoffManifest(opts, manifestRefs, pack, packMeta)
	if err := os.WriteFile(filepath.Join(opts.rootDir, "manifest.json"), data, 0o644); err != nil {
		return errors.Wrap(err, "write plugin handoff manifest")
	}
	return nil
}

// marshalPluginHandoffManifest renders manifest.json. The pack files are stored
// uncompressed beside it so a reader can fetch byte ranges of the artifact.
func marshalPluginHandoffManifest(
	opts pluginHandoffOptions,
	manifestRefs *fastjson.Value,
	pack, packMeta *pluginHandoffEntry,
) []byte {
	// Record the release identity and source provenance.
	var arena fastjson.Arena
	root := arena.NewObject()
	for _, field := range [][2]string{
		{"format", "plugin-handoff.v1"},
		{"plugin_rev", opts.pluginRev},
		{"release_environment", opts.releaseEnvironment},
		{"requested_selection", opts.requestedSelection},
		{"git_sha", opts.gitSHA},
		{"run_id", opts.runID},
		{"run_attempt", opts.runAttempt},
		{"source_repo", opts.sourceRepo},
		{"workflow", opts.workflow},
	} {
		root.Set(field[0], arena.NewString(field[1]))
	}

	// Record which surfaces were produced.
	surfaces := arena.NewObject()
	surfaces.Set("browser", pluginJSONBool(&arena, opts.includeBrowser))
	surfaces.Set("macos", pluginJSONBool(&arena, opts.includeMacOS))
	surfaces.Set("windows", pluginJSONBool(&arena, opts.includeWindows))
	surfaces.Set("linux", pluginJSONBool(&arena, opts.includeLinux))
	root.Set("produced_surfaces", surfaces)

	// Record the encoded manifest references and the pack files.
	root.Set("manifest_refs", manifestRefs)
	root.Set("pack", pluginHandoffEntryJSON(&arena, pack))
	root.Set("pack_metadata", pluginHandoffEntryJSON(&arena, packMeta))
	return append(root.MarshalTo(nil), '\n')
}

// pluginJSONBool returns a JSON boolean value.
func pluginJSONBool(arena *fastjson.Arena, v bool) *fastjson.Value {
	if v {
		return arena.NewTrue()
	}
	return arena.NewFalse()
}

// pluginHandoffEntryJSON returns the JSON object describing a handoff file.
func pluginHandoffEntryJSON(arena *fastjson.Arena, entry *pluginHandoffEntry) *fastjson.Value {
	// Describe the file by path, hash, and size.
	obj := arena.NewObject()
	obj.Set("path", arena.NewString(entry.Path))
	obj.Set("sha256", arena.NewString(entry.SHA256))
	obj.Set("size", arena.NewNumberString(strconv.FormatInt(entry.Size, 10)))
	return obj
}

// readPluginManifestRefs parses the manifest refs JSON array. The value is
// valid until parser parses again.
func readPluginManifestRefs(parser *fastjson.Parser, path string) (*fastjson.Value, error) {
	// Read the refs file.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "read manifest refs")
	}

	// Require a JSON array.
	v, err := parser.ParseBytes(data)
	if err != nil {
		return nil, errors.Wrap(err, "parse manifest refs")
	}
	if v.Type() != fastjson.TypeArray {
		return nil, errors.New("manifest refs must be a JSON array")
	}
	return v, nil
}

// buildPluginEntry describes a regular file under rootDir by its normalized
// relative path, hash, and size.
func buildPluginEntry(rootDir, filePath string) (*pluginHandoffEntry, error) {
	// Require a regular file, not a directory or symlink.
	info, err := os.Lstat(filePath)
	if err != nil {
		return nil, errors.Wrap(err, "stat "+filePath)
	}
	if info.IsDir() {
		return nil, errors.New("plugin handoff entry is a directory: " + filePath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("plugin handoff entry must not be a symlink: " + filePath)
	}

	// Require a normalized path inside the root.
	rel, err := filepath.Rel(rootDir, filePath)
	if err != nil {
		return nil, errors.Wrap(err, "rel plugin handoff path")
	}
	clean := path.Clean(filepath.ToSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(rel) {
		return nil, errors.New("plugin handoff entry escapes root: " + filePath)
	}
	if filepath.ToSlash(rel) != clean {
		return nil, errors.New("plugin handoff entry path is not normalized: " + filePath)
	}

	// Hash the file.
	sum, err := pluginFileSHA256(filePath)
	if err != nil {
		return nil, err
	}
	return &pluginHandoffEntry{
		Path:   clean,
		SHA256: sum,
		Size:   info.Size(),
	}, nil
}

// pluginFileSHA256 returns the hex SHA-256 of the file at path.
func pluginFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errors.Wrap(err, "open "+path)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", errors.Wrap(err, "hash "+path)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
