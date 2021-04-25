package forge_entitygraph

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/entitygraph/reporter"
	"github.com/aperturerobotics/entitygraph/store"
	"github.com/sirupsen/logrus"
)

// Reporter creates and handles directives, exposing entities to the graph.
type Reporter struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// store is the entitygraph store
	store *store.Store

	// mtx guards the refs list
	mtx sync.Mutex
	// cleanupRefs are the refs to cleanup
	cleanupRefs []directive.Reference
}

// NewReporter constructs a new Hydra entitygraph reporter.
func NewReporter(
	le *logrus.Entry,
	bus bus.Bus,
	store *store.Store,
) (reporter.Reporter, error) {
	return &Reporter{
		le:    le,
		bus:   bus,
		store: store,
	}, nil
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Reporter) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Reporter) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) (directive.Resolver, error) {
	return nil, nil
}

// _ is a type assertion
var _ reporter.Reporter = ((*Reporter)(nil))
