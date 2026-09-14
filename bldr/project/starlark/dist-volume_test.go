//go:build !js

package bldr_project_starlark

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDistVolumeConfig evaluates both supported protobuf field spellings and
// decodes the resulting configuration through the real distribution codec.
func TestDistVolumeConfig(t *testing.T) {
	for _, fields := range [][3]string{{"embedNativeVolume", "updateGuardPluginIds", "nativeRunnerPackage"}, {"embed_native_volume", "update_guard_plugin_ids", "native_runner_package"}} {
		field := fields[0]
		t.Run(field, func(t *testing.T) {
			// Evaluate the same typed constructor used by application projects.
			path := filepath.Join(t.TempDir(), "bldr.star")
			source := `project(id="volume-test")
manifest("desktop", builder="bldr/dist/compiler", config=dist_compiler_config(` + field + `="ENABLE", ` + fields[1] + `=["core"], ` + fields[2] + `="example.com/app/desktop"))
`
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := Evaluate(path)
			if err != nil {
				t.Fatal(err)
			}

			// The compiler receives the explicit embedded-volume selection.
			config := mustDistConfig(t, result.Config.GetManifests()["desktop"].GetBuilder().GetConfig())
			if !config.GetEmbedNativeVolume().IsEnabled(false) {
				t.Fatal("embedded-volume selection was lost during evaluation")
			}
			if ids := config.GetUpdateGuardPluginIds(); len(ids) != 1 || ids[0] != "core" {
				t.Fatalf("update guard selection was lost: %v", ids)
			}
			if config.GetNativeRunnerPackage() != "example.com/app/desktop" {
				t.Fatal("native runner selection was lost during evaluation")
			}
		})
	}
}
