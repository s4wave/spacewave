//go:build !js

package bldr_project_controller

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	frontend "github.com/s4wave/spacewave/bldr/frontend"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

// TestFrontendWatchWaitsForConfiguration keeps an early attachment from
// mistaking a not-yet-configured compiler for a disabled frontend.
func TestFrontendWatchWaitsForConfiguration(t *testing.T) {
	// Start a Watch stream before any compiler configuration exists.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	service := newFrontendService(nil, nil)
	stream := &frontendWatchStream{
		ctx: ctx, entered: make(chan struct{}, 1), events: make(chan *frontend.Event, 1),
	}
	returned := make(chan error, 1)
	go func() { returned <- service.Watch(&frontend.WatchRequest{}, stream) }()

	// Observe Watch entry before installing the compiler configuration.
	select {
	case <-stream.entered:
	case event := <-stream.events:
		t.Fatalf("early Watch sent a disabled session: %v", event)
	case err := <-returned:
		t.Fatalf("early Watch returned before configuration: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// A frontend module enables the compiler. The compiler's ready error
	// makes the post-configuration Watch path observable without launching
	// Vite.
	project := &bldr_project.ProjectConfig{}
	const projectJSON = `{"id":"colors","manifests":{"colors":{"builder":{"id":"bldr/plugin/compiler/js","config":{"modules":[{"kind":"JS_MODULE_KIND_FRONTEND","path":"./Viewer.ts"}]}}}}}`
	if err := bldr_project.UnmarshalProjectConfig([]byte(projectJSON), project); err != nil {
		t.Fatal(err)
	}
	if err := service.configure(&Config{SourcePath: "/source", WorkingPath: "/work", ProjectConfig: project}); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("compiler unavailable")
	service.ready.SetResult(nil, wantErr)

	// Watch must reach the configured compiler instead of sending a
	// disabled session.
	select {
	case event := <-stream.events:
		t.Fatalf("configured Watch sent a disabled session: %v", event)
	case err := <-returned:
		if !errors.Is(err, wantErr) {
			t.Fatalf("Watch returned %v, want %v", err, wantErr)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
