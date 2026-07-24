package app_web

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
)

func TestDistSourcesCoverManifestEntrypoints(t *testing.T) {
	manifest, err := os.ReadFile("../bldr.star")
	if err != nil {
		t.Fatal(err)
	}

	const marker = `web_pkg("@s4wave/web", entrypoints=[`
	start := strings.Index(string(manifest), marker)
	if start == -1 {
		t.Fatalf("bldr.star does not contain %q", marker)
	}
	surface := string(manifest)[start+len(marker):]
	before, _, ok := strings.Cut(surface, "])")
	if !ok {
		t.Fatal("@s4wave/web entrypoint list is not terminated")
	}

	entrypoints := strings.FieldsFunc(before, func(r rune) bool {
		switch r {
		case '"', ',', ' ', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
	if len(entrypoints) == 0 {
		t.Fatal("@s4wave/web has no entrypoints")
	}

	for _, entrypoint := range entrypoints {
		entrypoint = strings.TrimPrefix(entrypoint, "./")
		exists, err := embeddedEntrypointSource(entrypoint)
		if err != nil {
			t.Fatalf("inspect embedded entrypoint %s: %v", entrypoint, err)
		}
		if !exists {
			t.Errorf("DistSources has no source for @s4wave/web/%s", entrypoint)
		}
	}

	err = fs.WalkDir(DistSources, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && isWebTestSource(name) {
			t.Errorf("DistSources includes test source %s", name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDistSourcesAreClosed(t *testing.T) {
	importPattern, err := regexp.Compile(`(?m)(?:\bfrom\s*|\bimport\s*(?:\(\s*)?|@import\s*)["']([^"']+)["']`)
	if err != nil {
		t.Fatal(err)
	}

	err = fs.WalkDir(DistSources, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name == "test" {
				t.Error("DistSources includes the web/test helper directory")
			}
			return nil
		}
		if !isWebSource(name) {
			return nil
		}

		source, err := fs.ReadFile(DistSources, name)
		if err != nil {
			return err
		}
		for _, match := range importPattern.FindAllSubmatch(source, -1) {
			target, local := embeddedImportTarget(name, string(match[1]))
			if !local {
				continue
			}

			exists, err := embeddedSourceExists(target)
			if err != nil {
				return err
			}
			if !exists {
				t.Errorf("%s imports source %s, which is not embedded", name, match[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func embeddedImportTarget(source, importPath string) (string, bool) {
	const webPrefix = "@s4wave/web/"

	switch {
	case importPath == "@s4wave/web":
		return ".", true
	case strings.HasPrefix(importPath, webPrefix):
		return strings.TrimPrefix(importPath, webPrefix), true
	case strings.HasPrefix(importPath, "."):
		return path.Clean(path.Join(path.Dir(source), importPath)), true
	default:
		return "", false
	}
}

func embeddedSourceExists(target string) (bool, error) {
	var candidates []string
	switch path.Ext(target) {
	case ".js":
		base := strings.TrimSuffix(target, ".js")
		candidates = []string{base + ".ts", base + ".tsx"}
	case ".ts", ".tsx", ".css":
		candidates = []string{target}
	case "":
		candidates = []string{
			target + ".ts",
			target + ".tsx",
			path.Join(target, "index.ts"),
			path.Join(target, "index.tsx"),
		}
	default:
		candidates = []string{target}
	}

	for _, candidate := range candidates {
		entry, err := fs.Stat(DistSources, candidate)
		if err == nil {
			return !entry.IsDir(), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

func embeddedEntrypointSource(entrypoint string) (bool, error) {
	for _, ext := range []string{".ts", ".tsx", ".css"} {
		_, err := fs.Stat(DistSources, entrypoint+ext)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
	}

	entries, err := fs.ReadDir(DistSources, entrypoint)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && isWebSource(name) && !isWebTestSource(name) {
			return true, nil
		}
	}
	return false, nil
}

func isWebSource(name string) bool {
	return strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".tsx") || strings.HasSuffix(name, ".css")
}

func isWebTestSource(name string) bool {
	return strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".test.tsx")
}
