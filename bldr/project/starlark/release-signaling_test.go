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
	// Read the base manifest once for all environments.
	source, err := os.ReadFile("../../../bldr.star")
	if err != nil {
		t.Fatal(err)
	}

	// Evaluate each release environment's manifest with the release override.
	for _, env := range []string{"production", "staging"} {
		t.Run(env, func(t *testing.T) {
			// Append a release build target for the environment and evaluate it.
			path := filepath.Join(t.TempDir(), "bldr.star")
			build := "\n" + `build("release-env-test", manifests=["spacewave-core"], platform_ids=["js"], manifestOverrides=plugin_release_browser_manifest_overrides("spacewave-core", "` + env + `"))`
			if err := os.WriteFile(path, append(source, []byte(build)...), 0o644); err != nil {
				t.Fatal(err)
			}
			result, err := Evaluate(path)
			if err != nil {
				t.Fatal(err)
			}

			// Pick the environment's expected signaling and signing namespace.
			config := string(result.Config.GetBuild()["release-env-test"].GetManifestOverrides()["spacewave-core"].GetConfig())
			prefix, account := "spacewave", "https://account.spacewave.app"
			if env == "staging" {
				prefix, account = "spacewave-staging", "https://account-staging.spacewave.app"
			}

			// Assert the compiler config carries the environment's namespace.
			for _, want := range []string{`"signalingEnvPrefix":"` + prefix + `"`, `"signingEnvPrefix":"` + prefix + `"`, `"accountEndpoint":"` + account + `"`, `"signalingUrl":"/"`} {
				if !strings.Contains(config, want) {
					t.Fatalf("missing %s in %s", want, config)
				}
			}
		})
	}
}
