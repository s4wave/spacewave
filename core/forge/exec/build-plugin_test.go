//go:build !js && !tinygo

package space_exec

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/bldr/manifest/builder/resultworld"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// TestBuildSpacePlugin exercises Space source, the real Forge controller, and Bldr.
// The source contains no Go module or host checkout paths.
func TestBuildSpacePlugin(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	tb, sender := setupIntegrationTest(t, NewDefaultRegistry())
	const sourceKey = "projects/colors"
	const config = `{
  "id": "space-colors",
  "remotes": {"space": {"engineId": "wrong-world", "objectKey": "wrong-store"}},
  "manifests": {
    "space-colors": {
      "builder": {
        "id": "bldr/plugin/compiler/js",
        "config": {
          "webPkgs": [{"id": "@s4wave/web", "exclude": true}],
          "viteConfigPaths": ["vite.config.ts"],
          "viteDisableProjectConfig": true,
          "modules": [
            {"kind": "JS_MODULE_KIND_BACKEND", "path": "./backend.ts", "entrypoint": true},
            {"kind": "JS_MODULE_KIND_FRONTEND", "path": "./ColorViewer.tsx"}
          ]
        }
      }
    }
  }
}`
	createTestFS(t, ctx, tb.WorldState, sender, sourceKey, "bldr.yaml", []byte(config))
	// Compile the actual color app and viewer against the shipped SDK closure.
	files := map[string]string{
		"package.json":   `{"type":"module","dependencies":{"zod":"4.3.6"},"devDependencies":{"@vitejs/plugin-react":"6.0.5","vite":"8.2.2"}}`,
		"vite.config.ts": "import react from '@vitejs/plugin-react'\nexport default { plugins: [react()] }\n",
	}
	for _, name := range []string{"backend.ts", "app.ts", "ColorViewer.tsx"} {
		data, err := os.ReadFile(filepath.Join("../../../plugin/colors", name))
		if err != nil {
			t.Fatal(err)
		}
		source := strings.ReplaceAll(string(data), "../../sdk/", "@go/github.com/s4wave/spacewave/sdk/")
		files[name] = strings.ReplaceAll(source, "plugin/colors/ColorViewer.tsx", "./ColorViewer.tsx")
	}
	for name, source := range files {
		obj, err := world.MustGetObject(ctx, tb.WorldState, sourceKey)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = unixfs_world.FsMknodWithContent(ctx, obj, sender, unixfs_world.FSType_FSType_FS_NODE,
			[]string{name}, unixfs.NewFSCursorNodeType_File(), int64(len(source)), bytes.NewBufferString(source), 0o644, time.Now())
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}
	root, _, err := world.LookupRootRef(ctx, tb.Engine, sourceKey)
	if err != nil {
		t.Fatal(err)
	}
	sourceObj, err := world.MustGetObject(ctx, tb.WorldState, sourceKey)
	if err != nil {
		t.Fatal(err)
	}
	source, err := forge_value.NewWorldObjectSnapshot(ctx, sourceObj, tb.WorldState)
	world.ReleaseObjectState(sourceObj)
	if err != nil {
		t.Fatal(err)
	}
	createTestExecutionWithValueSet(t, ctx, tb.WorldState, sender, "build-colors", BuildPluginConfigID,
		[]byte(`{"manifest_id":"space-colors"}`), &forge_target.ValueSet{
			Inputs: forge_value.ValueSlice{forge_value.NewValueWithWorldObjectSnapshot("source", source)},
		})
	// Replace the editable tree after submission. The queued build must use its
	// captured input, even though the current directory no longer has a config.
	_, _, err = unixfs_world.FsInit(ctx, tb.WorldState, sender, sourceKey,
		unixfs_world.FSType_FSType_FS_NODE, nil, true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	execution := runTestExecution(t, tb, "build-colors", sender)
	assertComplete(t, execution)
	input := findOutput(execution, "source").GetWorldObjectSnapshot()
	if !input.GetRootRef().EqualVT(root) {
		t.Fatal("build did not retain the exact input source root")
	}
	artifact := findOutput(execution, "manifest").GetWorldObjectSnapshot()
	output := artifact.GetRootRef()
	if output == nil || findOutput(execution, "build-result").GetBucketRef() == nil {
		t.Fatal("build omitted the manifest or builder result")
	}
	provenance, _, err := resultworld.LookupManifestBuildResult(ctx, tb.WorldState, manifest.NewManifestArtifactKey(output))
	if err != nil {
		t.Fatal(err)
	}
	if provenance.GetSourceRef().GetBucketId() != "" || !provenance.GetSourceRef().GetRootRef().EqualVT(root.GetRootRef()) {
		t.Fatal("build provenance did not retain the exact source in its local bucket")
	}
	retained, err := block.ExtractBlockRefs(provenance)
	if err != nil {
		t.Fatal(err)
	}
	foundSource := false
	for _, ref := range retained {
		if ref.EqualVT(root.GetRootRef()) {
			foundSource = true
		}
	}
	if !foundSource {
		t.Fatal("build provenance did not expose the source root to garbage collection")
	}
	err = manifest_world.AccessManifest(ctx, tb.Logger, tb.WorldState.AccessWorldState, output,
		func(ctx context.Context, _ *bucket_lookup.Cursor, _ *block.Cursor, built *manifest.Manifest, dist, assets *unixfs.FSHandle) error {
			if built.GetMeta().GetManifestId() != "space-colors" {
				t.Fatal("build returned a different manifest")
			}
			entry, err := dist.Lookup(ctx, built.GetEntrypoint())
			if err != nil {
				return err
			}
			defer entry.Release()
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
}
