//go:build !js

package bldr_project_starlark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBrowserPluginReleaseEnvironment verifies the compiler receives matching
// cloud and standalone signing namespaces for each browser release environment.
func TestBrowserPluginReleaseEnvironment(t *testing.T) {
	source, err := os.ReadFile("../../../bldr.star")
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range []string{"production", "staging"} {
		t.Run(env, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bldr.star")
			build := "\n" + `build("release-env-test", manifests=["spacewave-core"], platform_ids=["js"], manifestOverrides=plugin_release_browser_manifest_overrides("spacewave-core", "` + env + `"))`
			if err := os.WriteFile(path, append(source, []byte(build)...), 0o644); err != nil {
				t.Fatal(err)
			}
			result, err := Evaluate(path)
			if err != nil {
				t.Fatal(err)
			}
			config := string(result.Config.GetBuild()["release-env-test"].GetManifestOverrides()["spacewave-core"].GetConfig())
			prefix, account := "spacewave", "https://account.spacewave.app"
			if env == "staging" {
				prefix, account = "spacewave-staging", "https://account-staging.spacewave.app"
			}
			for _, want := range []string{`"signalingEnvPrefix":"` + prefix + `"`, `"signingEnvPrefix":"` + prefix + `"`, `"accountEndpoint":"` + account + `"`, `"signalingUrl":"/"`} {
				if !strings.Contains(config, want) {
					t.Fatalf("missing %s in %s", want, config)
				}
			}
		})
	}
}
