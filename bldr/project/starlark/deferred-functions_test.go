//go:build !js

package bldr_project_starlark

import (
	"slices"
	"testing"
)

func TestBrowserReleaseOverridesKeepDeferredFunctions(t *testing.T) {
	// Evaluate the repository bldr.star manifest.
	result, err := Evaluate("../../../bldr.star")
	if err != nil {
		t.Fatal(err)
	}

	// Assert the core plugin keeps both deferred function boundaries.
	core := mustGoPluginConfig(t, result.Config.GetManifests()["spacewave-core"].GetBuilder().GetConfig())
	for _, fn := range []string{
		"github.com/s4wave/spacewave/core/git.LookupCreateGitRepoWizardOp",
		"github.com/s4wave/spacewave/sdk/world/wizard/resource.LookupWizardObjectType",
	} {
		if !slices.Contains(core.GetGoscriptDeferredFunctions(), fn) {
			t.Errorf("core plugin missing deferred boundary %s", fn)
		}
	}

	// Assert each release dist config keeps the v86 copy deferred boundary.
	for _, name := range []string{"release-web", "release-web-e2e-goscript"} {
		conf := mustDistConfig(t, result.Config.GetBuild()[name].GetManifestOverrides()["spacewave-browser"].GetConfig())
		for _, fn := range []string{
			"github.com/s4wave/spacewave/core/cdn/v86copy.CopyV86ImageFromCdnWithProgress",
		} {
			if !slices.Contains(conf.GetGoscriptDeferredFunctions(), fn) {
				t.Errorf("%s missing deferred boundary %s", name, fn)
			}
		}
	}
}
