//go:build !js

package devtool

import (
	"reflect"
	"testing"

	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

func TestProjectOwnedStartupPlugins(t *testing.T) {
	projectConfig := &bldr_project.ProjectConfig{
		Start: &bldr_project.StartConfig{
			Plugins: []string{"local-web", "external-web", "local-core"},
		},
		Manifests: map[string]*bldr_project.ManifestConfig{
			"local-core": {},
			"local-web":  {},
		},
	}

	got := projectOwnedStartupPlugins(projectConfig)
	want := []string{"local-web", "local-core"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectOwnedStartupPlugins() = %v, want %v", got, want)
	}
}

func TestProjectOwnedStartupPluginsEmpty(t *testing.T) {
	if got := projectOwnedStartupPlugins(&bldr_project.ProjectConfig{}); got != nil {
		t.Fatalf("projectOwnedStartupPlugins() = %v, want nil", got)
	}
}
