package auth_challenge_client

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/identity"
)

// lookupEntityResolver resolves lookup entity from network
type lookupEntityResolver struct {
	c   *Controller
	di  directive.Instance
	dir identity.IdentityLookupEntity
}

// resolveLookupEntity resolves a lookup entity from network directive.
func (c *Controller) resolveLookupEntity(
	ctx context.Context,
	di directive.Instance,
	dir identity.IdentityLookupEntity,
) (directive.Resolver, error) {
	return &lookupEntityResolver{
		c:   c,
		di:  di,
		dir: dir,
	}, nil
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (r *lookupEntityResolver) Resolve(
	ctx context.Context, handler directive.ResolverHandler,
) error {
	entityID := r.dir.IdentityLookupEntityID()
	r.c.le.Infof("not implemented: lookup entity: %v", entityID)
	return errors.New("not implemented lookup entity resolver")
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupEntityResolver)(nil))
