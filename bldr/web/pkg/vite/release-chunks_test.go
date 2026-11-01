//go:build !js

package web_pkg_vite

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	bldr_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	"github.com/sirupsen/logrus"
)

// TestWebPkgChunksKeepStableNames builds two releases through the production
// Vite service, including native StyleX CSS, and keeps lazy importers stable.
func TestWebPkgChunksKeepStableNames(t *testing.T) {
	// Build an ESM package with lazy modules and enough media queries to warm the parser.
	started := time.Now()
	ctx, cancel := context.WithTimeout(t.Context(), 75*time.Second)
	defer cancel()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	pkgRoot := t.TempDir()
	outDir := t.TempDir()
	for name, text := range map[string]string{
		"package.json": `{"name":"stable-chunks","type":"module"}`,
		"other.js":     `import * as stylex from '@stylexjs/stylex'; const styles=stylex.create({root:{backgroundColor:'blue',marginTop:7}}); export const attrs=stylex.props(styles.root)`,
		"lazy.js":      `export const value = 'first'`,
	} {
		if err := os.WriteFile(filepath.Join(pkgRoot, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Keep responsive exports live so the service exercises the optimized media parser.
	var entrySource strings.Builder
	entrySource.WriteString(`export const load = () => import('./lazy.js'); export const loadOther = () => import('./other.js');`)
	for index := range 32 {
		suffix := strconv.Itoa(index)
		name := "responsive" + suffix + ".js"
		entrySource.WriteString("export {attrs as responsive" + suffix + "} from './" + name + "';")
		text := `import * as stylex from '@stylexjs/stylex'; const styles=stylex.create({root:{gap:{default:'32px','@media (max-width: 960px)':'24px','@media (max-width: 720px)':'12px'}}}); export const attrs=stylex.props(styles.root);`
		if err := os.WriteFile(filepath.Join(pkgRoot, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(pkgRoot, "index.js"), []byte(entrySource.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run both release builds through the real bundler RPC.
	var firstEntry, firstLazy []byte
	var firstFiles []string
	runtimeDir := t.TempDir()
	workingPath, err := os.MkdirTemp(root, ".vite-release-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workingPath) })
	err = RunOneShot(ctx, logrus.NewEntry(logrus.New()), root, root, workingPath, func(ctx context.Context, client bldr_vite.SRPCViteBundlerClient) error {
		// Record service startup before compiling browser packages.
		t.Log("Vite service connected after", time.Since(started))

		// Build the same pinned ESM runtime that Bldr's import map serves to browser packages.
		runtime, err := client.BuildWebPkg(ctx, &bldr_vite.BuildWebPkgRequest{
			PkgId: "@stylexjs/stylex", PkgRoot: filepath.Join(root, "node_modules", "@stylexjs", "stylex"),
			Imports: []string{"lib/es/stylex.mjs"}, OutDir: runtimeDir, IsRelease: true,
		})
		if err != nil {
			return err
		}
		if !runtime.GetSuccess() {
			return errors.New(runtime.GetError())
		}
		var runtimePath string
		for _, entry := range runtime.GetImportMapEntries() {
			if entry.GetSpecifier() == "@stylexjs/stylex" {
				runtimePath = entry.GetOutputPath()
			}
		}
		if runtimePath == "" {
			return errors.New("StyleX runtime has no bare import-map entry")
		}

		// Build successive lazy-module revisions against the same entry and package graph.
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
			for _, file := range wantFiles {
				if !slices.Contains(files, file) {
					return errors.Errorf("missing stable entry %s in %v", file, files)
				}
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
			if !bytes.Contains(css, []byte("stylex.priority3000")) || !bytes.Contains(css, []byte("color:red")) || !bytes.Contains(css, []byte("margin-top:7px")) || strings.Contains(string(lazy), "stylex.create") {
				return errors.Errorf("StyleX did not compile through native package CSS: %s", css)
			}
			if !bytes.Contains(css, []byte("gap:32px")) || !bytes.Contains(css, []byte("960px")) || !bytes.Contains(css, []byte("720px")) {
				return errors.Errorf("responsive exports lost their native media rules: %s", css)
			}

			// A chunk edit changes only its bytes, not its importing module.
			if value == "first" {
				t.Log("Responsive package built after", time.Since(started))
				if os.Getenv("STYLEX_WEBPKG_BROWSER") == "1" {
					if err := checkWebPkgBrowser(ctx, root, outDir, runtimeDir, runtimePath); err != nil {
						return err
					}
					t.Log("Both browser engines completed after", time.Since(started))
				}
				firstEntry, firstLazy, firstFiles = entry, lazy, files
				continue
			}
			if !slices.Equal(firstFiles, files) || !bytes.Equal(firstEntry, entry) || bytes.Equal(firstLazy, lazy) {
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
			// The theme remains a cache input even when its JavaScript exports are eliminated.
			if !slices.Contains(resp.GetSourceFiles(), filepath.Join(root, "web", "style", "tokens.stylex.ts")) {
				return errors.Errorf("%s omitted the compiler-read theme from package provenance", source)
			}

			manifest, err := os.ReadFile(filepath.Join(outDir, ".vite", "manifest.json"))
			if err != nil {
				return err
			}
			if !bytes.Contains(manifest, []byte(".css")) {
				return errors.Errorf("%s did not emit native CSS", source)
			}
			cssFiles, err := filepath.Glob(filepath.Join(outDir, "*.css"))
			if err != nil {
				return err
			}
			if len(cssFiles) != 1 {
				return errors.Errorf("%s emitted %d CSS assets, want one native package sheet", source, len(cssFiles))
			}
		}
		if os.Getenv("STYLEX_WEBPKG_BROWSER") == "1" {
			return checkSharedButtonBrowser(ctx, root, client)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// checkWebPkgBrowser exercises native browser ESM imports while their actual stylesheet request is held.
func checkWebPkgBrowser(ctx context.Context, root, outDir, runtimeDir, runtimePath string) error {
	// Serve the built package and pinned runtime from the same disposable browser origin.
	html := `<!doctype html><html><body><div id="target" data-state="loading">Loading</div><div id="other"></div><script type="importmap">{"imports":{"@stylexjs/stylex":"/runtime/` + runtimePath + `"}}</script><script type="module">import {load,loadOther} from './index.mjs'; const [{attrs},other]=await Promise.all([load(),loadOther()]);Object.assign(document.getElementById('other'),other.attrs);const target=document.getElementById('target');Object.assign(target,attrs);target.dataset.state='ready';target.textContent='Ready';</script></body></html>`
	if err := os.WriteFile(filepath.Join(outDir, "index.html"), []byte(html), 0o644); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/runtime/", http.StripPrefix("/runtime/", http.FileServer(http.Dir(runtimeDir))))
	mux.Handle("/", http.FileServer(http.Dir(outDir)))
	server := httptest.NewServer(mux)
	defer server.Close()

	// Node's installed Playwright owns Chromium and WebKit independently of playwright-go.
	command := exec.CommandContext(ctx, "node", filepath.Join(root, "scripts", "stylex-web-pkg.browser.ts"), server.URL)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "native web package browser load: %s", output)
	}
	return nil
}

// checkSharedButtonBrowser loads controls and caller icons from one native shared package.
func checkSharedButtonBrowser(ctx context.Context, root string, client bldr_vite.SRPCViteBundlerClient) error {
	// Keep every generated package in one disposable build root.
	directory, err := os.MkdirTemp(root, ".stylex-button-rpc-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)

	// Use the production owner for CJS wrappers, sibling links and import-map names.
	packageOut := filepath.Join(directory, "packages")
	refs := []*web_pkg.WebPkgRef{
		{WebPkgId: "react", WebPkgRoot: filepath.Join(root, "node_modules/react"), Imports: []string{"index.js", "jsx-runtime.js"}},
		{WebPkgId: "react-dom", WebPkgRoot: filepath.Join(root, "node_modules/react-dom"), Imports: []string{"index.js", "client.js"}},
		{WebPkgId: "@stylexjs/stylex", WebPkgRoot: filepath.Join(root, "node_modules/@stylexjs/stylex"), Imports: []string{"lib/es/stylex.mjs"}},
		{WebPkgId: "@s4wave/web", WebPkgRoot: filepath.Join(root, "web"), Imports: []string{"ui/button.tsx", "images/icons.ts"}},
	}
	_, _, entries, err := BuildWebPkgsVite(ctx, logrus.NewEntry(logrus.New()), root, refs, packageOut, "/b/pkg/", true, true, false, client, filepath.Join(directory, "cache"))
	if err != nil {
		return err
	}

	// Serve the exact paths returned by the real package build.
	mux := http.NewServeMux()
	mux.Handle("/b/pkg/", http.StripPrefix("/b/pkg/", http.FileServer(http.Dir(packageOut))))

	// The consumer owns its icon element while the shared button owns its inherited rule.
	appRoot := filepath.Join(directory, "app")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		return err
	}
	source := `import React from 'react';import{createRoot}from'react-dom/client';import{Button}from'@s4wave/web/ui/button.js';import{LuPlus,IconContext}from'@s4wave/web/images/icons.js';createRoot(document.getElementById('target')).render(<IconContext.Provider value={{color:'rgb(17, 34, 51)'}}><Button id="native-button"><LuPlus/>Native bundle action</Button></IconContext.Provider>);`
	if err := os.WriteFile(filepath.Join(appRoot, "App.tsx"), []byte(source), 0o644); err != nil {
		return err
	}

	// Compile the consumer with the shared package kept external.
	appOut := filepath.Join(directory, "consumer")
	response, err := client.BuildWebPkg(ctx, &bldr_vite.BuildWebPkgRequest{PkgId: "native-button-consumer", PkgRoot: appRoot, Imports: []string{"App.tsx"}, ExternalPkgs: []string{"react", "react-dom", "@stylexjs/stylex", "@s4wave/web"}, OutDir: appOut, WebPkgBasePath: "/b/pkg/", IsRelease: true})
	if err != nil {
		return err
	}
	if !response.GetSuccess() {
		return errors.New(response.GetError())
	}

	// Install the actual returned import map before the consumer starts.
	var arena fastjson.Arena
	imports := arena.NewObject()
	for _, entry := range entries {
		imports.Set(entry.Specifier, arena.NewString(entry.OutputPath))
	}
	mapping := arena.NewObject()
	mapping.Set("imports", imports)
	html := `<!doctype html><html><body><div id="target" data-state="loading">Loading</div><script type="importmap">` + string(mapping.MarshalTo(nil)) + `</script><script type="module">await import('./App.mjs');document.getElementById('target').dataset.state='ready';</script></body></html>`
	if err := os.WriteFile(filepath.Join(appOut, "index.html"), []byte(html), 0o644); err != nil {
		return err
	}

	// Keep the HTTP origin alive until both browser engines finish.
	mux.Handle("/", http.FileServer(http.Dir(appOut)))
	server := httptest.NewServer(mux)
	defer server.Close()

	// Hold the real shared stylesheet before accepting a rendered cross-package icon.
	command := exec.CommandContext(ctx, "node", filepath.Join(root, "scripts", "stylex-web-pkg.browser.ts"), server.URL, "buttons")
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "native shared button browser: %s", output)
	}
	return nil
}
