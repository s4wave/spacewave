//go:build !js

package bldr_project_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	frontend "github.com/s4wave/spacewave/bldr/frontend"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

// frontendWatchStream captures the first response and announces Watch entry.
type frontendWatchStream struct {
	srpc.Stream
	// ctx is the lifetime of this attachment.
	ctx context.Context
	// entered reports that Watch acquired the stream context.
	entered chan struct{}
	// events receives responses sent by Watch.
	events chan *frontend.Event
}

// Context reports when Watch has entered before returning its lifetime.
func (s *frontendWatchStream) Context() context.Context {
	select {
	case s.entered <- struct{}{}:
	default:
	}
	return s.ctx
}

// Send records an attachment response.
func (s *frontendWatchStream) Send(event *frontend.Event) error {
	s.events <- event
	return nil
}

// SendAndClose records the final attachment response.
func (s *frontendWatchStream) SendAndClose(event *frontend.Event) error {
	return s.Send(event)
}

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

	// A frontend module enables the compiler; its ready error makes the
	// post-configuration Watch path observable without launching Vite.
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

	// Watch must reach the configured compiler instead of sending disabled.
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

// _ is a type assertion.
var _ frontend.SRPCFrontend_WatchStream = (*frontendWatchStream)(nil)
