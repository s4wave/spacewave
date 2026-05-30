//go:build !js

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

type pluginHandoffOptions struct {
	rootDir            string
	worldPath          string
	manifestRefsPath   string
	nativeDir          string
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

type pluginHandoffEntry struct {
	Path   string
	SHA256 string
	Size   int64
}

func runWritePluginHandoffManifest(args []string) error {
	fs := flag.NewFlagSet("write-handoff-manifest", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts pluginHandoffOptions
	if err := func() error {
		fs.StringVar(&opts.rootDir, "root", "", "path to the plugin handoff root")
		fs.StringVar(&opts.worldPath, "world", "", "path to devtool.s4wave")
		fs.StringVar(&opts.manifestRefsPath, "manifest-refs", "", "path to manifest refs JSON")
		fs.StringVar(&opts.nativeDir, "native-dir", "", "path to native artifact directory")
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
	return writePluginHandoffManifest(opts)
}

func writePluginHandoffManifest(opts pluginHandoffOptions) error {
	if opts.rootDir == "" {
		return errors.New("handoff root dir is required")
	}
	if opts.worldPath == "" {
		return errors.New("world path is required")
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
	manifestRefs, err := readPluginManifestRefs(opts.manifestRefsPath)
	if err != nil {
		return err
	}
	world, err := buildPluginHandoffEntry(opts.rootDir, opts.worldPath)
	if err != nil {
		return errors.Wrap(err, "collect plugin world")
	}
	if world.Path != "devtool.s4wave" {
		return errors.New("plugin world path must be devtool.s4wave")
	}
	artifacts, err := collectPluginHandoffArtifacts(opts.nativeDir)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(opts.rootDir, "manifest.json"), []byte(marshalPluginHandoffManifest(opts, world, artifacts, manifestRefs)), 0o644); err != nil {
		return errors.Wrap(err, "write plugin handoff manifest")
	}
	return nil
}

func readPluginManifestRefs(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", errors.Wrap(err, "read manifest refs")
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		return "", errors.Wrap(err, "parse manifest refs")
	}
	if v.Type() != fastjson.TypeArray {
		return "", errors.New("manifest refs must be a JSON array")
	}
	return strings.TrimSpace(string(data)), nil
}

func collectPluginHandoffArtifacts(nativeDir string) ([]*pluginHandoffEntry, error) {
	if nativeDir == "" {
		return nil, nil
	}
	if _, err := os.Stat(nativeDir); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrap(err, "stat native artifacts")
	}
	var out []*pluginHandoffEntry
	if err := filepath.WalkDir(nativeDir, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return errors.Wrap(err, "walk "+filePath)
		}
		if entry.IsDir() {
			return nil
		}
		file, err := buildPluginArtifactEntry(nativeDir, filePath)
		if err != nil {
			return err
		}
		out = append(out, file)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out, nil
}

func buildPluginHandoffEntry(rootDir, filePath string) (*pluginHandoffEntry, error) {
	return buildPluginEntry(rootDir, filePath)
}

func buildPluginArtifactEntry(nativeDir, filePath string) (*pluginHandoffEntry, error) {
	return buildPluginEntry(nativeDir, filePath)
}

func buildPluginEntry(rootDir, filePath string) (*pluginHandoffEntry, error) {
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

func marshalPluginHandoffManifest(opts pluginHandoffOptions, world *pluginHandoffEntry, artifacts []*pluginHandoffEntry, manifestRefs string) string {
	var b strings.Builder
	b.WriteString("{\n")
	writePluginJSONField(&b, 1, "format", "plugin-handoff.v1", true)
	writePluginJSONField(&b, 1, "plugin_rev", opts.pluginRev, true)
	writePluginJSONField(&b, 1, "release_environment", opts.releaseEnvironment, true)
	writePluginJSONField(&b, 1, "requested_selection", opts.requestedSelection, true)
	writePluginJSONField(&b, 1, "git_sha", opts.gitSHA, true)
	writePluginJSONField(&b, 1, "run_id", opts.runID, true)
	writePluginJSONField(&b, 1, "run_attempt", opts.runAttempt, true)
	writePluginJSONField(&b, 1, "source_repo", opts.sourceRepo, true)
	writePluginJSONField(&b, 1, "workflow", opts.workflow, true)
	b.WriteString("  \"produced_surfaces\": {\n")
	writePluginJSONBoolField(&b, 2, "browser", opts.includeBrowser, true)
	writePluginJSONBoolField(&b, 2, "macos", opts.includeMacOS, true)
	writePluginJSONBoolField(&b, 2, "windows", opts.includeWindows, true)
	writePluginJSONBoolField(&b, 2, "linux", opts.includeLinux, false)
	b.WriteString("  },\n")
	b.WriteString("  \"manifest_refs\": ")
	b.WriteString(manifestRefs)
	b.WriteString(",\n")
	b.WriteString("  \"world\": ")
	writePluginHandoffEntry(&b, world)
	b.WriteString(",\n")
	b.WriteString("  \"artifacts\": [\n")
	for i, artifact := range artifacts {
		b.WriteString("    ")
		writePluginHandoffEntry(&b, artifact)
		if i != len(artifacts)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ]\n")
	b.WriteString("}\n")
	return b.String()
}

func writePluginJSONField(b *strings.Builder, indent int, name, value string, trailing bool) {
	b.WriteString(strings.Repeat("  ", indent))
	b.WriteString(strconv.Quote(name))
	b.WriteString(": ")
	b.WriteString(strconv.Quote(value))
	if trailing {
		b.WriteByte(',')
	}
	b.WriteByte('\n')
}

func writePluginJSONBoolField(b *strings.Builder, indent int, name string, value bool, trailing bool) {
	b.WriteString(strings.Repeat("  ", indent))
	b.WriteString(strconv.Quote(name))
	b.WriteString(": ")
	b.WriteString(strconv.FormatBool(value))
	if trailing {
		b.WriteByte(',')
	}
	b.WriteByte('\n')
}

func writePluginHandoffEntry(b *strings.Builder, entry *pluginHandoffEntry) {
	b.WriteString("{")
	b.WriteString(strconv.Quote("path"))
	b.WriteString(": ")
	b.WriteString(strconv.Quote(entry.Path))
	b.WriteString(", ")
	b.WriteString(strconv.Quote("sha256"))
	b.WriteString(": ")
	b.WriteString(strconv.Quote(entry.SHA256))
	b.WriteString(", ")
	b.WriteString(strconv.Quote("size"))
	b.WriteString(": ")
	b.WriteString(strconv.FormatInt(entry.Size, 10))
	b.WriteString("}")
}
