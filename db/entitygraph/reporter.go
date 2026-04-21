package hydra_entitygraph

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/entitygraph/reporter"
	"github.com/aperturerobotics/entitygraph/store"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/peer"
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

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Reporter) Execute(ctx context.Context) error {
	c.le.Info("registering LookupVolume directive")
	_, diRef2, err := c.bus.AddDirective(
		volume.NewLookupVolume("", peer.ID("")),
		newLookupVolumeHandler(c),
	)
	if err != nil {
		return err
	}
	defer diRef2.Release()

	// Wait for the controller to quit
	<-ctx.Done()

	// Cleanup all created refs
	c.mtx.Lock()
	for _, ref := range c.cleanupRefs {
		ref.Release()
	}
	c.cleanupRefs = nil
	c.mtx.Unlock()
	return ctx.Err()
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Reporter) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// _ is a type assertion
var _ reporter.Reporter = ((*Reporter)(nil))
