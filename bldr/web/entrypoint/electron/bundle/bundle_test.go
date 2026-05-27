//go:build !js

package entrypoint_electron_bundle

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bldr "github.com/s4wave/spacewave/bldr"
	"github.com/sirupsen/logrus"
)

func TestBuildRendererBundleUsesSelfContainedDistEntrypoint(t *testing.T) {
	appRoot := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(appRoot, "go.mod"),
		[]byte("module github.com/example/downstream\n\ngo 1.26\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(appRoot, "tsconfig.json"),
		[]byte(`{"compilerOptions":{"paths":{"@go/*":["./vendor/*"]}}}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	distRoot := filepath.Join(appRoot, ".bldr", "src")
	copyDistSources(t, distRoot)

	buildDir := filepath.Join(appRoot, "build")
	if err := BuildRendererBundle(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		distRoot,
		buildDir,
		"",
		"sw.mjs",
		"shw.mjs",
		"",
		false,
		true,
	); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(filepath.Join(buildDir, "entrypoint", "entrypoint.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, unexpected := range []string{
		"@s4wave/web/router/app-path.js",
		"@s4wave/app/prerender/boot-status.js",
	} {
		if strings.Contains(string(out), unexpected) {
			t.Fatalf("output still contains unresolved import %q", unexpected)
		}
	}
}

func copyDistSources(t *testing.T, dest string) {
	t.Helper()
	if err := fs.WalkDir(bldr.DistSources, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := bldr.DistSources.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}); err != nil {
		t.Fatal(err)
	}
}
