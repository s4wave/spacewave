package bifrost_entitygraph

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport"

	"github.com/aperturerobotics/entitygraph/reporter"
	"github.com/aperturerobotics/entitygraph/store"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"

	"github.com/sirupsen/logrus"
)

// Reporter creates and handles directives, exposing entities to the graph.
// It handles EstablishLink directives to observe running and pending links.
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

// NewReporter constructs a new Bifrost entitygraph reporter.
// emits two node Entity objects per EstablishLink interest (node objects)
// emits one Entity<Link> object per EstablishLink interest (not value, just interest)
// emits one Entity<Link> object per known Link yielded by EstablishLink
// emits one Transport object per known remote transport (from Link objects)
// emits one Transport object per known local transport (from LookupTransport directives)
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
	c.le.Debug("registering lookuptransport directive")
	_, diRef1, err := c.bus.AddDirective(
		transport.NewLookupTransport(peer.ID(""), 0),
		newLookupTransportHandler(c),
	)
	if err != nil {
		return err
	}
	defer diRef1.Release()

	c.le.Debug("registering get peer directive")
	_, diRef2, err := c.bus.AddDirective(
		peer.NewGetPeer(peer.ID("")),
		newGetPeerHandler(c),
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
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Reporter) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case link.EstablishLinkWithPeer:
		c.handleEstablishLink(ctx, di, d)
	}

	return nil, nil
}

// handleEstablishLink handles an EstablishLink directive.
func (c *Reporter) handleEstablishLink(
	ctx context.Context,
	di directive.Instance,
	d link.EstablishLinkWithPeer,
) {
	handler := newEstablishLinkHandler(c)
	ref := di.AddReference(handler, true)
	if ref == nil {
		return
	}
	handler.ref = ref

	c.mtx.Lock()
	c.cleanupRefs = append(c.cleanupRefs, ref)
	c.mtx.Unlock()
}

// _ is a type assertion
var _ reporter.Reporter = ((*Reporter)(nil))
