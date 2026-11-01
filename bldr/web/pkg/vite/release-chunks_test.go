//go:build !js

package web_pkg_vite

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	bldr_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	"github.com/sirupsen/logrus"
)

// TestWebPkgChunksKeepStableNames builds two releases through the production
// Vite service, including native StyleX CSS, and keeps lazy importers stable.
func TestWebPkgChunksKeepStableNames(t *testing.T) {
	// Build an ESM package with one lazy module.
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	pkgRoot := t.TempDir()
	outDir := t.TempDir()
	for name, text := range map[string]string{
		"package.json": `{"name":"stable-chunks","type":"module"}`,
		"index.js":     `export const load = () => import('./lazy.js')`,
		"lazy.js":      `export const value = 'first'`,
	} {
		if err := os.WriteFile(filepath.Join(pkgRoot, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Run both release builds through the real bundler RPC.
	var firstEntry, firstLazy []byte
	workingPath, err := os.MkdirTemp(root, ".vite-release-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workingPath) })
	err = RunOneShot(ctx, logrus.NewEntry(logrus.New()), root, root, workingPath, func(ctx context.Context, client bldr_vite.SRPCViteBundlerClient) error {
		for _, value := range []string{"first", "second"} {
			if err := os.WriteFile(filepath.Join(pkgRoot, "lazy.js"), []byte("import * as stylex from '@stylexjs/stylex'; const styles = stylex.create({root:{color:'red',padding:4}}); export const attrs = stylex.props(styles.root); export const value = '"+value+"'"), 0o644); err != nil {
				return err
			}
			resp, err := client.BuildWebPkg(ctx, &bldr_vite.BuildWebPkgRequest{
				PkgId: "stable-chunks", PkgRoot: pkgRoot, Imports: []string{"index.js"},
				ExternalPkgs: []string{"@stylexjs/stylex"},
				OutDir:       outDir, WebPkgBasePath: "/b/pkg/", IsRelease: true, JsMinification: true,
			})
			if err != nil {
				return err
			}
			if !resp.GetSuccess() {
				return errors.New(resp.GetError())
			}

			// Require stable entry and lazy chunk names in each release.
			files, err := filepath.Glob(filepath.Join(outDir, "*.mjs"))
			if err != nil {
				return err
			}
			wantFiles := []string{filepath.Join(outDir, "index.mjs"), filepath.Join(outDir, "lazy.mjs")}
			if !slices.Equal(files, wantFiles) {
				return errors.Errorf("module files = %v, want %v", files, wantFiles)
			}
			entry, err := os.ReadFile(wantFiles[0])
			if err != nil {
				return err
			}
			lazy, err := os.ReadFile(wantFiles[1])
			if err != nil {
				return err
			}

			// Native CSS reaches the package asset graph after compiling the moved service.
			cssFiles, err := filepath.Glob(filepath.Join(outDir, "*.css"))
			if err != nil {
				return err
			}
			if len(cssFiles) != 1 {
				return errors.Errorf("CSS assets = %v, want one native asset", cssFiles)
			}
			css, err := os.ReadFile(cssFiles[0])
			if err != nil {
				return err
			}
			if !bytes.Contains(css, []byte("stylex.priority")) || !bytes.Contains(css, []byte("color:red")) || strings.Contains(string(lazy), "stylex.create") {
				return errors.New("StyleX did not compile through native package CSS")
			}

			// A chunk edit changes only its bytes, not its importing module.
			if value == "first" {
				firstEntry, firstLazy = entry, lazy
				continue
			}
			if !bytes.Equal(firstEntry, entry) || bytes.Equal(firstLazy, lazy) {
				return errors.New("lazy chunk change rewrote its importer or left the chunk unchanged")
			}
		}

		// Real shared, app and plugin modules use the same RPC and distribution layout.
		for _, source := range []string{"web/ui/InfoCard.tsx", "app/vm/VmErrorBanner.tsx", "plugin/notes/blog/TagChip.tsx"} {
			resp, err := client.BuildWebPkg(ctx, &bldr_vite.BuildWebPkgRequest{
				PkgId: "stylex-pilots", PkgRoot: root, Imports: []string{source},
				ExternalPkgs: []string{"@stylexjs/stylex", "react", "react-icons"},
				OutDir:       outDir, WebPkgBasePath: "/b/pkg/", IsRelease: true,
			})
			if err != nil {
				return err
			}
			if !resp.GetSuccess() {
				return errors.Errorf("%s: %s", source, resp.GetError())
			}
			manifest, err := os.ReadFile(filepath.Join(outDir, ".vite", "manifest.json"))
			if err != nil {
				return err
			}
			if !bytes.Contains(manifest, []byte(".css")) {
				return errors.Errorf("%s did not emit native CSS", source)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
