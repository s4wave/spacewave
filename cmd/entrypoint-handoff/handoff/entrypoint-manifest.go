//go:build !js

package handoff

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// EntrypointHandoffOptions carries inputs for writing an entrypoint release handoff.
type EntrypointHandoffOptions struct {
	RootDir            string
	BrowserStagingDir  string
	StaticManifestPath string
	Version            string
	Rev                string
	GitSHA             string
	Tag                string
	ReleaseEnvironment string
	RunID              string
	RunAttempt         string
	SourceRepo         string
	Workflow           string
}

type entrypointHandoffManifest struct {
	browserStaging []*entrypointHandoffEntry
	staticManifest *entrypointHandoffEntry
	opts           EntrypointHandoffOptions
}

type entrypointHandoffEntry struct {
	Path   string
	SHA256 string
	Size   int64
}

// WriteEntrypointHandoffManifest writes manifest.json for an entrypoint handoff.
func WriteEntrypointHandoffManifest(opts EntrypointHandoffOptions) error {
	if opts.RootDir == "" {
		return errors.New("handoff root dir is required")
	}
	if opts.BrowserStagingDir == "" {
		return errors.New("browser staging dir is required")
	}
	if opts.StaticManifestPath == "" {
		return errors.New("static manifest path is required")
	}
	if opts.Version == "" {
		return errors.New("version is required")
	}
	if opts.Rev == "" {
		return errors.New("rev is required")
	}
	if opts.ReleaseEnvironment == "" {
		return errors.New("release environment is required")
	}
	if opts.GitSHA == "" || opts.RunID == "" || opts.RunAttempt == "" || opts.SourceRepo == "" || opts.Workflow == "" {
		return errors.New("source provenance is required")
	}

	browserEntries, err := collectEntrypointHandoffFiles(opts.RootDir, opts.BrowserStagingDir)
	if err != nil {
		return errors.Wrap(err, "collect browser staging files")
	}
	if len(browserEntries) == 0 {
		return errors.New("browser staging files are required")
	}
	staticEntry, err := buildEntrypointHandoffEntry(opts.RootDir, opts.StaticManifestPath)
	if err != nil {
		return errors.Wrap(err, "collect static manifest")
	}
	if staticEntry.Path != "static-manifest.ts" {
		return errors.New("static manifest path must be static-manifest.ts")
	}
	manifest := &entrypointHandoffManifest{
		browserStaging: browserEntries,
		staticManifest: staticEntry,
		opts:           opts,
	}
	if err := os.WriteFile(filepath.Join(opts.RootDir, "manifest.json"), []byte(marshalEntrypointHandoffManifest(manifest)), 0o644); err != nil {
		return errors.Wrap(err, "write entrypoint handoff manifest")
	}
	return nil
}

func collectEntrypointHandoffFiles(rootDir, dir string) ([]*entrypointHandoffEntry, error) {
	var out []*entrypointHandoffEntry
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return errors.Wrap(err, "walk "+path)
		}
		if entry.IsDir() {
			return nil
		}
		file, err := buildEntrypointHandoffEntry(rootDir, path)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(file.Path, "browser-staging/") {
			return errors.New("browser staging file escaped browser-staging: " + file.Path)
		}
		out = append(out, file)
		return nil
	}); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b *entrypointHandoffEntry) int {
		return strings.Compare(a.Path, b.Path)
	})
	return out, nil
}

func buildEntrypointHandoffEntry(rootDir, filePath string) (*entrypointHandoffEntry, error) {
	info, err := os.Lstat(filePath)
	if err != nil {
		return nil, errors.Wrap(err, "stat "+filePath)
	}
	if info.IsDir() {
		return nil, errors.New("handoff entry is a directory: " + filePath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("handoff entry must not be a symlink: " + filePath)
	}
	rel, err := filepath.Rel(rootDir, filePath)
	if err != nil {
		return nil, errors.Wrap(err, "rel handoff path")
	}
	clean := path.Clean(filepath.ToSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(rel) {
		return nil, errors.New("handoff entry escapes root: " + filePath)
	}
	if filepath.ToSlash(rel) != clean {
		return nil, errors.New("handoff entry path is not normalized: " + filePath)
	}
	digest, err := fileSHA256(filePath)
	if err != nil {
		return nil, err
	}
	return &entrypointHandoffEntry{
		Path:   clean,
		SHA256: digest,
		Size:   info.Size(),
	}, nil
}

func marshalEntrypointHandoffManifest(manifest *entrypointHandoffManifest) string {
	var b strings.Builder
	opts := manifest.opts
	b.WriteString("{\n")
	writeEntrypointJSONField(&b, 1, "format", "entrypoint-handoff.v1", true)
	writeEntrypointJSONField(&b, 1, "release_environment", opts.ReleaseEnvironment, true)
	writeEntrypointJSONField(&b, 1, "version", opts.Version, true)
	writeEntrypointJSONField(&b, 1, "rev", opts.Rev, true)
	writeEntrypointJSONField(&b, 1, "tag", opts.Tag, true)
	writeEntrypointJSONField(&b, 1, "git_sha", opts.GitSHA, true)
	writeEntrypointJSONField(&b, 1, "run_id", opts.RunID, true)
	writeEntrypointJSONField(&b, 1, "run_attempt", opts.RunAttempt, true)
	writeEntrypointJSONField(&b, 1, "source_repo", opts.SourceRepo, true)
	writeEntrypointJSONField(&b, 1, "workflow", opts.Workflow, true)
	b.WriteString("  \"browser_staging\": [\n")
	for i, entry := range manifest.browserStaging {
		b.WriteString("    ")
		writeEntrypointHandoffEntry(&b, entry)
		if i != len(manifest.browserStaging)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ],\n")
	b.WriteString("  \"static_manifest\": ")
	writeEntrypointHandoffEntry(&b, manifest.staticManifest)
	b.WriteString("\n}\n")
	return b.String()
}

func writeEntrypointJSONField(b *strings.Builder, indent int, name, value string, trailing bool) {
	b.WriteString(strings.Repeat("  ", indent))
	b.WriteString(strconv.Quote(name))
	b.WriteString(": ")
	b.WriteString(strconv.Quote(value))
	if trailing {
		b.WriteByte(',')
	}
	b.WriteByte('\n')
}

func writeEntrypointHandoffEntry(b *strings.Builder, entry *entrypointHandoffEntry) {
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
