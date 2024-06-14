package forge_entitygraph

import (
	"context"

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

// Execute executes the controller goroutine.
func (c *Reporter) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Reporter) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// _ is a type assertion
var _ reporter.Reporter = ((*Reporter)(nil))
