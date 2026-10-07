//go:build !js

package bldr_project_controller

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	frontend "github.com/s4wave/spacewave/bldr/frontend"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
)

// TestFrontendBootstrapAttachesStartup keeps editable startup sources out of the
// immutable renderer while retaining the refresh preamble before its import.
func TestFrontendBootstrapAttachesStartup(t *testing.T) {
	// Configure the application and its startup view through project manifests.
	project := &bldr_project.ProjectConfig{}
	const projectJSON = `{"id":"app","start":{"loadWebStartup":"./app/startup.tsx"},"manifests":{"app":{"builder":{"id":"bldr/plugin/compiler/js","config":{"modules":[{"kind":"JS_MODULE_KIND_FRONTEND","path":"./app/App.tsx"}]}}}}}`
	if err := bldr_project.UnmarshalProjectConfig([]byte(projectJSON), project); err != nil {
		t.Fatal(err)
	}
	service := newFrontendService(nil, nil)
	if err := service.configure(&Config{SourcePath: "/source", WorkingPath: "/work", ProjectConfig: project}); err != nil {
		t.Fatal(err)
	}
	if entries := service.run.GetState().GetEntrypoints(); !slices.Equal(entries, []string{"app/App.tsx", "app/startup.tsx"}) {
		t.Fatalf("frontend entries = %v", entries)
	}

	// Publish a ready compiler capability without starting a native process.
	service.ready.SetResult(&frontendEnvironment{result: &vite.DevelopmentResult{
		Session: &frontend.Session{Id: "first", RoutePrefix: "/b/fe/first/"},
	}}, nil)
	response := httptest.NewRecorder()
	service.ServeBootstrap(response, httptest.NewRequest(http.MethodGet, "/bldr-dev/frontend-boot.mjs", nil), "entrypoint/entrypoint.mjs", "app/startup.tsx")

	// The refresh import precedes the immutable renderer and supplies a stable source path.
	want := `import "/bldr-dev/frontend-refresh/first.mjs"; window.__bldrFrontendEnabled = true; window.__bldrFrontendStartup = "app/startup.tsx"; await import("/entrypoint/entrypoint.mjs");`
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("bootstrap response = %d %q", response.Code, response.Body.String())
	}
}
