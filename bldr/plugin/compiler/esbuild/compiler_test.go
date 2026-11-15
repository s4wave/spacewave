package esbuild

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/aperturerobotics/util/enabled"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	build_policy "github.com/s4wave/spacewave/bldr/manifest/build"
	builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	"github.com/s4wave/spacewave/bldr/project/bunlock"
	validate "github.com/s4wave/spacewave/bldr/project/validate"
	"github.com/s4wave/spacewave/bldr/testbed"
	bundler "github.com/s4wave/spacewave/bldr/web/bundler"
	bundle "github.com/s4wave/spacewave/bldr/web/bundler/esbuild"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store/inmem"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	"github.com/sirupsen/logrus"
)

// TestBuildManifestFromPersistedBlocks builds a plugin after registry shutdown,
// then reads its manifest, chunks and frontend through production interfaces.
func TestBuildManifestFromPersistedBlocks(t *testing.T) {
	// Serve pinned npm packages, including a scoped package and a nested dependency version.
	tarballs := map[string][]byte{
		"answer": npmArchive(t, map[string]string{
			"package.json": `{"name":"answer","version":"1.0.0","main":"index.cjs","scripts":{"install":"exit 99"}}`,
			"index.cjs":    `exports.answer = require('helper') + 40; exports.shared = [require('react'), require('react/jsx-runtime'), require('@aptre/bldr-sdk/hooks/useResource')]`,
		}),
		"helper":        npmArchive(t, map[string]string{"index.js": `module.exports = 1000`}),
		"answer/helper": npmArchive(t, map[string]string{"index.js": `module.exports = 2`}),
		"spacewave": npmArchive(t, map[string]string{
			"package.json": `{"name":"spacewave","version":"1.0.0","exports":{"./react":{"browser":"./react.js","default":"./wrong.js"}}}`,
			"react.js":     `export {useResource} from '@aptre/bldr-sdk/hooks/useResource'; export const bundled = "bundled-spacewave"`,
			"wrong.js":     `throw new Error('wrong export condition')`,
		}),
		"@acme/util": npmArchive(t, map[string]string{
			"package.json":     `{"name":"@acme/util","version":"1.0.0","exports":{"./*":"./lib/*.js","./feature/*":{"browser":{"development":"./wrong.js"},"import":[null,"./feature/*.js"],"default":"./wrong.js"}}}`,
			"feature/value.js": `export const scopedValue = 'scoped-block-value'`,
			"feature/value.ts": `export const scopedValue = 'wrong-npm-typescript-neighbor'`,
		}),
	}
	var lock strings.Builder
	lock.WriteString(`{"lockfileVersion":1,"packages":{`)
	for _, key := range slices.Sorted(maps.Keys(tarballs)) {
		sum := sha512.Sum512(tarballs[key])
		name := key
		if key == "answer/helper" {
			name = "helper"
		}
		lock.WriteString(strconv.Quote(key) + ":[" + strconv.Quote(name+"@1.0.0") + "," + strconv.Quote("http://registry.test/"+key) + `,{},` + strconv.Quote("sha512-"+base64.StdEncoding.EncodeToString(sum[:])) + "],")
	}
	lock.WriteString("}}")
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if _, err := w.Write(tarballs[strings.TrimPrefix(request.URL.Path, "/")]); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(registry.Close)
	lockData := []byte(strings.ReplaceAll(lock.String(), "http://registry.test", registry.URL))

	// Open a writable dependency index backed by the in-memory block store.
	store := block_store.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_inmem.NewStore(), 0)
	_, root := block.NewTransaction(store, nil, nil, nil)
	root.SetBlock(kvtx_block.NewKeyValueStore(0), true)
	dependencies, err := kvtx_block.BuildKvTransaction(t.Context(), root, true)
	if err != nil {
		t.Fatal(err)
	}

	// Fetch pinned bytes without evaluating the packages' install scripts.
	fetcher, err := bunlock.NewFetcher(registry.Client(), registry.URL, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fetcher.Fetch(t.Context(), lockData, dependencies); err != nil {
		t.Fatal(err)
	}

	// Persist both the index and tarball blocks before releasing the first transaction.
	if err := dependencies.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	ref, _, err := root.GetTransaction().Write(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	dependencies.Discard()

	// Reopen the persisted dependency tree after removing all registry access.
	registry.Close()
	_, root = block.NewTransaction(store, nil, ref, nil)
	dependencies, err = kvtx_block.BuildKvTransaction(t.Context(), root, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dependencies.Discard)
	files := fstest.MapFS{
		"bldr.star": {Data: []byte(`
project(id="block-fixture")
manifest("block-plugin", builder="bldr/plugin/compiler/js", description="Pinned block fixture", rev=3, config={
    "webPluginId": "web",
    "modules": [
        js_module("JS_MODULE_KIND_BACKEND", "./plugin.ts", entrypoint=True),
        js_module("JS_MODULE_KIND_FRONTEND", "./frontend.tsx", entrypoint=True),
    ],
    "webPkgs": [{"id": "@custom/shared", "exclude": True}],
})
`)},
		"package.json": {Data: []byte(`{"name":"block-fixture","dependencies":{"answer":"1.0.0","spacewave":"1.0.0","@acme/util":"1.0.0"}}`)},
		"bun.lock":     {Data: lockData},
		"plugin.ts":    {Data: []byte(`import {answer, shared} from 'answer'; export const value: number = answer; export {shared}; export default async function plugin(): Promise<void> { console.info(value) }`)},
		"frontend.tsx": {Data: []byte(`
import React from 'react'
import {createRoot} from 'react-dom/client'
import {bundled, useResource} from 'spacewave/react'
import {isBackendEntrypointFunc} from '@aptre/bldr-sdk'
import {shared} from '@custom/shared'
import {scopedValue} from '@acme/util/feature/value'
import {value, fallback} from './value.js'
import {exact} from './exact.js'
import {appended} from './appended.js'
import {jsxFallback} from './rewrite.jsx'
import {moduleFallback} from './module.mjs'
import {commonFallback} from './common.cjs'
import './style.css'
import icon from './icon.svg'
export {icon, React, createRoot, bundled, useResource, isBackendEntrypointFunc, value, fallback, exact, appended, jsxFallback, moduleFallback, commonFallback, shared, scopedValue}
export const element = <span>{bundled}</span>
export const lazy = () => import('./lazy.ts')
`)},
		"value.ts":       {Data: []byte(`export const value: number = 7; export const fallback = 'source-js-fallback'`)},
		"exact.js":       {Data: []byte(`export const exact = 'source-exact-js'`)},
		"exact.ts":       {Data: []byte(`export const exact = 'wrong-source-typescript-neighbor'`)},
		"appended.js.ts": {Data: []byte(`export const appended = 'source-appended-extension'`)},
		"appended.ts":    {Data: []byte(`export const appended = 'wrong-source-rewrite-before-appended'`)},
		"rewrite.ts":     {Data: []byte(`export const jsxFallback = 'source-jsx-fallback'`)},
		"rewrite.tsx":    {Data: []byte(`export const jsxFallback = 'wrong-source-tsx-before-ts'`)},
		"module.mts":     {Data: []byte(`export const moduleFallback = 'source-mjs-fallback'`)},
		"common.cts":     {Data: []byte(`export const commonFallback = 'source-cjs-fallback'`)},
		"lazy.ts":        {Data: []byte(`export const lazyValue: string = 'lazy-block-value'`)},
		"style.css":      {Data: []byte(`span { color: red }`)},
		"icon.svg":       {Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)},
	}
	source := persistedSource(t, files)

	// Start a Bldr World with no executable tools available to the build.
	t.Setenv("PATH", t.TempDir())
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.BuildTestbed(t.Context(), le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Configure the backend and frontend through the existing manifest target interface.
	compiler, err := NewCompiler(le, tb.GetBus(), source, dependencies, &Config{
		Entrypoint: "plugin.ts", MaxExpandedSize: 1 << 20,
		Assets:  []*bundle.EsbuildBundleEntrypoint{{InputPath: "frontend.tsx", OutputPath: "frontend"}},
		WebPkgs: []*bundler.WebPkgRefConfig{{Id: "@custom/shared", Exclude: true}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Select the testbed World as the manifest's publication destination.
	args := &builder.BuildManifestArgs{BuilderConfig: &builder.BuilderConfig{
		ManifestMeta: manifest.NewManifestMeta("block-plugin", manifest.BuildType_DEV, "js", 1),
		EngineId:     tb.GetWorldEngineID(), PeerId: tb.GetVolume().GetPeerID().String(),
		LinkObjectKeys: []string{tb.GetPluginHostObjKey()},
		BuildPolicy:    &build_policy.BuildPolicy{JsMinification: enabled.Enabled_DISABLE, JsSourcemaps: enabled.Enabled_DISABLE},
	}}
	var target builder.Controller = compiler

	// A canceled build cannot publish a partial plugin.
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := target.BuildManifest(canceled, args, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled build = %v", err)
	}

	// Build from reopened source and dependency blocks after registry shutdown.
	result, err := target.BuildManifest(t.Context(), args, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}

	// Inspect immutable output through the same manifest and UnixFS APIs as the host.
	err = manifest_world.AccessManifest(t.Context(), le, tb.GetWorldState().AccessWorldState, result.GetManifestRef().GetManifestRef(), func(ctx context.Context, bls *bucket_lookup.Cursor, bcs *block.Cursor, value *manifest.Manifest, dist, assets *unixfs.FSHandle) error {
		// Check the bundled CommonJS dependency selected its nested version.
		code, err := fs.ReadFile(unixfs_iofs.NewFS(ctx, dist), value.GetEntrypoint())
		if err != nil {
			return err
		}
		if !strings.Contains(string(code), "module.exports = 2") || strings.Contains(string(code), "1000") || strings.Contains(string(code), `from "answer"`) {
			return errors.New("plugin did not bundle the pinned nested dependency")
		}
		if strings.Contains(string(code), "Dynamic require of") || !strings.Contains(string(code), `from "react/jsx-runtime"`) || !strings.Contains(string(code), `from "@aptre/bldr-sdk/hooks/useResource"`) {
			return errors.New("CommonJS shared imports did not become singleton ESM imports")
		}

		// Check shared package roots and subpaths survive while Spacewave is bundled.
		output := unixfs_iofs.NewFS(ctx, assets)
		frontend, err := fs.ReadFile(output, "frontend.mjs")
		if err != nil {
			return err
		}
		for _, want := range []string{`from "react"`, `from "react-dom/client"`, `from "@aptre/bldr-sdk"`, `from "@aptre/bldr-sdk/hooks/useResource"`, `from "react/jsx-runtime"`, `from "@custom/shared"`, `bundled-spacewave`, `scoped-block-value`} {
			if !strings.Contains(string(frontend), want) {
				return errors.Errorf("frontend is missing %s", want)
			}
		}
		if strings.Contains(string(frontend), `from "spacewave`) || strings.Contains(string(frontend), "wrong export condition") {
			return errors.New("Spacewave browser export was not bundled")
		}

		// Explicit files precede appended extensions, which precede TypeScript rewrites.
		for _, want := range []string{"source-exact-js", "source-appended-extension", "source-js-fallback", "source-jsx-fallback", "source-mjs-fallback", "source-cjs-fallback"} {
			if !strings.Contains(string(frontend), want) {
				return errors.Errorf("frontend is missing extension resolution result %s", want)
			}
		}
		if strings.Contains(string(frontend), "wrong-source-") || strings.Contains(string(frontend), "wrong-npm-typescript-neighbor") {
			return errors.New("extension resolution selected an incorrect TypeScript neighbor")
		}

		// Imported images become emitted files rather than parsed JavaScript.
		if !strings.Contains(string(frontend), "assets/icon-") {
			return errors.New("svg import was not emitted as an asset")
		}

		// Read all emitted assets, including CSS and lazy chunks, from addressed blocks.
		if _, err := fs.ReadFile(output, "frontend.css"); err != nil {
			return err
		}
		chunks, err := fs.ReadDir(output, "chunks")
		if err != nil || len(chunks) == 0 {
			return errors.New("lazy import has no persisted chunk")
		}
		return fs.WalkDir(output, ".", func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return walkErr
			}
			_, err := fs.ReadFile(output, name)
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	// Preserve source provenance without treating dependency archive paths as source.
	var inputs []string
	for _, file := range result.GetInputManifest().GetFiles() {
		inputs = append(inputs, file.GetPath())
	}
	if !slices.Equal(inputs, []string{"appended.js.ts", "bun.lock", "common.cts", "exact.js", "frontend.tsx", "icon.svg", "lazy.ts", "module.mts", "plugin.ts", "rewrite.ts", "style.css", "value.ts"}) {
		t.Fatalf("source provenance = %v", inputs)
	}

	// Remove verified dependency blocks and refuse publication of an incomplete build.
	_, emptyRoot := block.NewTransaction(store, nil, nil, nil)
	emptyRoot.SetBlock(kvtx_block.NewKeyValueStore(0), true)
	empty, err := kvtx_block.BuildKvTransaction(t.Context(), emptyRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Discard()
	missing, err := NewCompiler(le, tb.GetBus(), source, empty, compiler.config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := missing.BuildManifest(t.Context(), args, nil); err == nil || !strings.Contains(err.Error(), "verified npm tarball is missing") {
		t.Fatalf("missing dependency build = %v", err)
	}

	// Only the complete successful build should be linked in the World.
	retained, err := manifest_world.ListManifests(t.Context(), tb.GetWorldState(), tb.GetPluginHostObjKey())
	if err != nil {
		t.Fatal(err)
	}
	if len(retained) != 1 {
		t.Fatalf("published %d manifests, want one complete build", len(retained))
	}

	// Build the shared repository entrypoint and module assets directly.
	t.Run("repository", func(t *testing.T) {
		// Use the repository's evaluated module configuration and manifest metadata.
		project, _, err := validate.ValidateProject(t.Context(), unixfs_iofs.NewFS(t.Context(), source))
		if err != nil {
			t.Fatal(err)
		}
		selected := project.GetManifests()["block-plugin"]
		config := &js.Config{}
		if err := config.UnmarshalJSON(selected.GetBuilder().GetConfig()); err != nil {
			t.Fatal(err)
		}
		conf := args.GetBuilderConfig().CloneVT()
		conf.ManifestMeta.Description = selected.GetDescription()
		conf.ManifestMeta.Rev = selected.GetRev()

		// Build offline using the existing dependency blocks and manifest interface.
		target, err := NewRepositoryCompiler(t.Context(), le, tb.GetBus(), source, dependencies, config, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := target.Close(); err != nil {
				t.Error(err)
			}
		})
		result, err := target.BuildManifest(t.Context(), &builder.BuildManifestArgs{BuilderConfig: conf}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := result.Validate(); err != nil {
			t.Fatal(err)
		}

		// Read the generated lifecycle wrapper and both compiled module assets.
		err = manifest_world.AccessManifest(t.Context(), le, tb.GetWorldState().AccessWorldState, result.GetManifestRef().GetManifestRef(), func(ctx context.Context, bls *bucket_lookup.Cursor, bcs *block.Cursor, value *manifest.Manifest, dist, assets *unixfs.FSHandle) error {
			// Preserve the reviewed description and revision in the committed manifest.
			if value.GetMeta().GetDescription() != "Pinned block fixture" || value.GetMeta().GetRev() != 3 {
				return errors.New("repository manifest lost its reviewed metadata")
			}

			// Use Bldr's shared entrypoint lifecycle instead of treating a module as the host entrypoint.
			code, err := fs.ReadFile(unixfs_iofs.NewFS(ctx, dist), value.GetEntrypoint())
			if err != nil {
				return err
			}
			for _, want := range []string{"createPluginEntrypoint", "@aptre/bldr/plugin-entrypoint", "/assets/modules/0.mjs", "modules/1.mjs", "modules/1.css"} {
				if !strings.Contains(string(code), want) {
					return errors.Errorf("repository entrypoint is missing %s", want)
				}
			}

			// Keep verified npm code bundled while shared React and SDK imports remain external.
			output := unixfs_iofs.NewFS(ctx, assets)
			backend, err := fs.ReadFile(output, "modules/0.mjs")
			if err != nil {
				return err
			}
			frontend, err := fs.ReadFile(output, "modules/1.mjs")
			if err != nil {
				return err
			}
			if !strings.Contains(string(backend), "module.exports = 2") || !strings.Contains(string(frontend), "bundled-spacewave") || !strings.Contains(string(frontend), `from "react"`) {
				return errors.New("repository modules did not use pinned dependencies and shared imports")
			}
			_, err = fs.ReadFile(output, "modules/1.css")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		// Record the reviewed repository files alongside the module source provenance.
		var inputs []string
		for _, file := range result.GetInputManifest().GetFiles() {
			inputs = append(inputs, file.GetPath())
		}
		if !slices.Contains(inputs, "bldr.star") || !slices.Contains(inputs, "package.json") || !slices.Contains(inputs, "bun.lock") {
			t.Fatalf("repository provenance = %v", inputs)
		}
	})
}

// npmArchive creates a deterministic npm tarball from regular package files.
func npmArchive(t *testing.T, files map[string]string) []byte {
	// Encode source and metadata using npm's package/ archive layout.
	t.Helper()
	var output bytes.Buffer
	compressed := gzip.NewWriter(&output)
	archive := tar.NewWriter(compressed)
	for _, name := range slices.Sorted(maps.Keys(files)) {
		contents := files[name]
		if err := archive.WriteHeader(&tar.Header{Name: "package/" + name, Mode: 0o644, Size: int64(len(contents))}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}

	// Finish the compressed archive before it is hashed by the lock fixture.
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

// persistedSource copies fixture files to UnixFS and reopens the stored root.
func persistedSource(t *testing.T, files fs.FS) *unixfs.FSHandle {
	// Open the real block-backed filesystem testbed.
	t.Helper()
	tb, err := db_testbed.NewTestbed(t.Context(), logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Persist the source snapshot in UnixFS before opening its reader.
	cursor, err := tb.BuildEmptyCursor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	tx, root := cursor.BuildTransaction(nil)
	if err := unixfs_block.CreateFromFS(t.Context(), root, files, nil); err != nil {
		t.Fatal(err)
	}
	ref, _, err := tx.Write(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	cursor.SetRootRef(ref)

	// Read source from persisted blocks with no writable filesystem or fixture map.
	stored := unixfs_fs.NewFS(t.Context(), unixfs_block.NodeType_NodeType_DIRECTORY, cursor, nil)
	handle, err := unixfs.NewFSHandle(stored)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(handle.Release)
	return handle
}
