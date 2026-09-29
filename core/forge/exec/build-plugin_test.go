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
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/testbed"
)

// spaceColorsConfig is the Bldr project of the colors plugin built from a Space.
const spaceColorsConfig = `{
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

// writeSpaceColorsFiles writes the colors app and viewer, rewritten to import
// the shipped SDK closure, into the Space directory at sourceKey.
func writeSpaceColorsFiles(t *testing.T, ctx context.Context, tb *testbed.Testbed, sender peer.ID, sourceKey string) {
	// Read the actual plugin sources and point their SDK imports at the closure.
	t.Helper()
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

	// Write each file into the Space directory.
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
}

// assertBuildProvenance checks the build result retains the exact source root
// in its local bucket and exposes it to garbage collection.
func assertBuildProvenance(t *testing.T, ctx context.Context, tb *testbed.Testbed, output *bucket.ObjectRef, root *bucket.ObjectRef) {
	// Look up the result stored beside the artifact, reporting failures to the caller.
	t.Helper()
	provenance, _, err := resultworld.LookupManifestBuildResult(ctx, tb.WorldState, manifest.NewManifestArtifactKey(output))
	if err != nil {
		t.Fatal(err)
	}
	if provenance.GetSourceRef().GetBucketId() != "" || !provenance.GetSourceRef().GetRootRef().EqualVT(root.GetRootRef()) {
		t.Fatal("build provenance did not retain the exact source in its local bucket")
	}

	// Check the source root is among the references the result retains.
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
}

// TestBuildSpacePlugin exercises Space source, the real Forge controller, and Bldr.
// The source contains no Go module or host checkout paths.
func TestBuildSpacePlugin(t *testing.T) {
	// Write the colors project into a Space directory.
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	tb, sender := setupIntegrationTest(t, NewDefaultRegistry())
	const sourceKey = "projects/colors"
	createTestFS(t, ctx, tb.WorldState, sender, sourceKey, "bldr.yaml", []byte(spaceColorsConfig))
	writeSpaceColorsFiles(t, ctx, tb, sender, sourceKey)

	// Pin the directory's current root as the build input.
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

	// Queue the build with an explicit platform and capacity request.
	createTestExecutionWithValueSet(t, ctx, tb.WorldState, sender, "build-colors", BuildPluginConfigID,
		[]byte(`{"manifest_id":"space-colors","platform_id":"js","milli_cpu":1000,"memory_bytes":1073741824}`), &forge_target.ValueSet{
			Inputs: forge_value.ValueSlice{forge_value.NewValueWithWorldObjectSnapshot("source", source)},
		})

	// Replace the editable tree after submission. The queued build must use its
	// captured input, even though the current directory no longer has a config.
	_, _, err = unixfs_world.FsInit(ctx, tb.WorldState, sender, sourceKey,
		unixfs_world.FSType_FSType_FS_NODE, nil, true, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Run the build and check it retained the exact input and outputs.
	execution := runTestExecution(t, tb, "build-colors", sender)
	assertComplete(t, execution)
	input := findOutput(execution, "source").GetWorldObjectSnapshot()
	if !input.GetRootRef().EqualVT(root) {
		t.Fatal("build did not retain the exact input source root")
	}
	output := findOutput(execution, "manifest").GetWorldObjectSnapshot().GetRootRef()
	if output == nil || findOutput(execution, "build-result").GetBucketRef() == nil {
		t.Fatal("build omitted the manifest or builder result")
	}
	assertBuildProvenance(t, ctx, tb, output, root)

	// Open the built manifest and check its identity and entrypoint.
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
