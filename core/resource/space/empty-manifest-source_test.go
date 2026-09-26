package resource_space

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// emptyManifestSource serves an empty FetchManifest value for one manifest ID
// and reports the first fetch.
type emptyManifestSource struct {
	// manifestID is the served manifest ID.
	manifestID string
	// started closes on the first fetch.
	started chan struct{}
	// once closes started.
	once sync.Once
}

// newEmptyManifestSource constructs a manifest source for manifestID.
func newEmptyManifestSource(manifestID string) *emptyManifestSource {
	return &emptyManifestSource{manifestID: manifestID, started: make(chan struct{})}
}

// GetControllerInfo returns information about the controller.
func (c *emptyManifestSource) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/empty-manifest-source",
		controller.MustParseVersion("0.0.1"),
		"serves an empty manifest",
	)
}

// Execute executes the controller.
func (c *emptyManifestSource) Execute(context.Context) error {
	return nil
}

// HandleDirective serves FetchManifest for the manifest ID.
func (c *emptyManifestSource) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(bldr_manifest.FetchManifest)
	if !ok || dir.GetManifestId() != c.manifestID {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		c.once.Do(func() { close(c.started) })
		_, _ = handler.AddValue(&bldr_manifest.FetchManifestValue{})
		handler.MarkIdle(true)
		<-ctx.Done()
		return ctx.Err()
	}), nil)
}

// Close releases any resources used by the controller.
func (c *emptyManifestSource) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*emptyManifestSource)(nil)
