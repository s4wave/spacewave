//go:build !js

package bldr_project_validate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// jsProject is a bldr.star declaring one JavaScript plugin.
const jsProject = `
project(id="acme")
manifest("acme-colors",
    builder="bldr/plugin/compiler/js",
    description="Color swatches",
    config={
        "webPluginId": "web",
        "modules": [
            js_module("JS_MODULE_KIND_BACKEND", "./plugin/backend.ts", entrypoint=True),
            js_module("JS_MODULE_KIND_FRONTEND", "./plugin/ColorViewer.tsx"),
        ],
    },
)
`

// packageJSON names one direct dependency.
const packageJSON = `{"name": "acme", "dependencies": {"left-pad": "^1.3.0"}}`

// bunLock pins the direct dependency and one nested package by hash, with the
// trailing commas Bun writes.
const bunLock = `{
  "lockfileVersion": 1,
  "workspaces": {
    "": {
      "name": "acme",
      "dependencies": {
        "left-pad": "^1.3.0",
      },
    },
  },
  "packages": {
    "left-pad": ["left-pad@1.3.0", "", {"dependencies": {"@acme/util": "1.0.0"}}, "sha512-left,"],
    "@acme/util": ["@acme/util@1.0.0", "", {}, "sha512-util"],
  }
}
`

// acceptedFiles is a JavaScript plugin repository the validation accepts.
func acceptedFiles() fstest.MapFS {
	return fstest.MapFS{
		"bldr.star":                   {Data: []byte(jsProject)},
		"package.json":                {Data: []byte(packageJSON)},
		"bun.lock":                    {Data: []byte(bunLock)},
		"plugin/backend.ts":           {Data: []byte("export default {}\n")},
		"plugin/ColorViewer.tsx":      {Data: []byte("export default {}\n")},
		"node_modules/.keep":          {Data: nil},
		"plugin/unrelated/readme.txt": {Data: []byte("hi\n")},
	}
}

// TestValidateAcceptsJsPlugin shows the plugin and its pinned dependencies.
func TestValidateAcceptsJsPlugin(t *testing.T) {
	// Validate the accepted repository.
	v, err := Validate(context.Background(), acceptedFiles())
	if err != nil {
		t.Fatal(err)
	}
	if len(v.GetRefusals()) != 0 {
		t.Fatalf("unexpected refusals: %v", v.GetRefusals())
	}

	// Check the plugin and its modules.
	if len(v.GetPlugins()) != 1 {
		t.Fatalf("expected one plugin, got %v", v.GetPlugins())
	}
	plugin := v.GetPlugins()[0]
	if plugin.GetManifestId() != "acme-colors" || plugin.GetDescription() != "Color swatches" || len(plugin.GetModules()) != 2 {
		t.Fatalf("unexpected plugin: %v", plugin)
	}
	if plugin.GetModules()[0].GetPath() != "./plugin/backend.ts" {
		t.Fatalf("unexpected module: %v", plugin.GetModules()[0])
	}

	// Check both locked packages in name order, with only left-pad direct.
	deps := v.GetDependencies()
	if len(deps) != 2 {
		t.Fatalf("expected two dependencies, got %v", deps)
	}
	if deps[0].GetName() != "@acme/util" || deps[0].GetVersion() != "1.0.0" || deps[0].GetDirect() {
		t.Fatalf("unexpected nested dependency: %v", deps[0])
	}
	if deps[1].GetName() != "left-pad" || deps[1].GetVersion() != "1.3.0" || !deps[1].GetDirect() {
		t.Fatalf("unexpected direct dependency: %v", deps[1])
	}
}

// TestValidateRefusals refuses each rule with its kind and reason.
func TestValidateRefusals(t *testing.T) {
	cases := []struct {
		// name names the case.
		name string
		// edit changes the accepted repository.
		edit func(fstest.MapFS)
		// kind is the expected refusal kind.
		kind RefusalKind
		// reason is a substring of the expected reason.
		reason string
	}{{
		name: "go plugin",
		edit: func(files fstest.MapFS) {
			files["bldr.star"] = &fstest.MapFile{Data: []byte(jsProject + `
manifest("acme-native", builder="bldr/plugin/compiler/go", config={"goPkgs": ["./native"]})
`)}
		},
		kind:   RefusalKind_REFUSAL_KIND_GO_PLUGIN,
		reason: `Plugin "acme-native" is a Go plugin`,
	}, {
		name: "vite config file",
		edit: func(files fstest.MapFS) {
			files["vite.config.ts"] = &fstest.MapFile{Data: []byte("export default {}\n")}
		},
		kind:   RefusalKind_REFUSAL_KIND_VITE_CONFIG,
		reason: "vite.config.ts",
	}, {
		name: "vite config path in a platform override",
		edit: func(files fstest.MapFS) {
			files["bldr.star"] = &fstest.MapFile{Data: []byte(strings.Replace(jsProject, `"webPluginId": "web",`,
				`"webPluginId": "web", "platformTypes": {"js": {"viteConfigPaths": ["./build.ts"]}},`, 1))}
		},
		kind:   RefusalKind_REFUSAL_KIND_VITE_CONFIG,
		reason: "sets a Vite config",
	}, {
		name: "lifecycle script",
		edit: func(files fstest.MapFS) {
			files["package.json"] = &fstest.MapFile{Data: []byte(`{"scripts": {"postinstall": "node x.js"}, "dependencies": {"left-pad": "^1.3.0"}}`)}
		},
		kind:   RefusalKind_REFUSAL_KIND_LIFECYCLE_SCRIPT,
		reason: "postinstall",
	}, {
		name: "unpinned git dependency",
		edit: func(files fstest.MapFS) {
			files["bun.lock"] = &fstest.MapFile{Data: []byte(strings.Replace(bunLock,
				`"@acme/util": ["@acme/util@1.0.0", "", {}, "sha512-util"],`,
				`"@acme/util": ["@acme/util@github:acme/util#abc123", {}, "acme-util-abc123"],`, 1))}
		},
		kind:   RefusalKind_REFUSAL_KIND_UNPINNED_DEPENDENCY,
		reason: "@acme/util@github:acme/util#abc123",
	}, {
		name: "missing lockfile",
		edit: func(files fstest.MapFS) {
			delete(files, "bun.lock")
		},
		kind:   RefusalKind_REFUSAL_KIND_UNPINNED_DEPENDENCY,
		reason: "no bun.lock",
	}, {
		name: "load outside the repository",
		edit: func(files fstest.MapFS) {
			files["bldr.star"] = &fstest.MapFile{Data: []byte(`load("../outside.star", "VALUE")` + jsProject)}
		},
		kind:   RefusalKind_REFUSAL_KIND_CONFIG,
		reason: "path escapes project root",
	}, {
		name: "load outside vendor",
		edit: func(files fstest.MapFS) {
			files["bldr.star"] = &fstest.MapFile{Data: []byte(`load("@go/../../outside.star", "VALUE")` + jsProject)}
		},
		kind:   RefusalKind_REFUSAL_KIND_CONFIG,
		reason: "path escapes vendor root",
	}, {
		name: "module outside the repository",
		edit: func(files fstest.MapFS) {
			files["bldr.star"] = &fstest.MapFile{Data: []byte(strings.Replace(jsProject, "./plugin/backend.ts", "../backend.ts", 1))}
		},
		kind:   RefusalKind_REFUSAL_KIND_CONFIG,
		reason: `names module "../backend.ts" outside the repository`,
	}, {
		name: "remote",
		edit: func(files fstest.MapFS) {
			files["bldr.star"] = &fstest.MapFile{Data: []byte(jsProject + `
remote("devtool", engineId="e", objectKey="k")
`)}
		},
		kind:   RefusalKind_REFUSAL_KIND_CONFIG,
		reason: "build, remote or publish",
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Validate the edited repository.
			files := acceptedFiles()
			tc.edit(files)
			v, err := Validate(context.Background(), files)
			if err != nil {
				t.Fatal(err)
			}

			// Require the expected refusal.
			for _, refusal := range v.GetRefusals() {
				if refusal.GetKind() == tc.kind && strings.Contains(refusal.GetReason(), tc.reason) {
					return
				}
			}
			t.Fatalf("expected a %v refusal containing %q, got %v", tc.kind, tc.reason, v.GetRefusals())
		})
	}
}

// TestValidateStopsEndlessEvaluation stops a bldr.star that never finishes
// when the context ends.
func TestValidateStopsEndlessEvaluation(t *testing.T) {
	// Validate a project whose evaluation loops forever.
	files := acceptedFiles()
	files["bldr.star"] = &fstest.MapFile{Data: []byte("while True:\n    pass\n")}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := Validate(ctx, files)

	// Require the context's error.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the deadline error, got %v", err)
	}
}
