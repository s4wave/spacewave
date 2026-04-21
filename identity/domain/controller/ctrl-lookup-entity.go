package identity_domain_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/identity"
)

// lookupEntityResolver resolves a lookup entity directive
type lookupEntityResolver struct {
	c   *Controller
	ctx context.Context
	dir identity.IdentityLookupEntity
}

// Resolve resolves the values, emitting them to the handler.
func (o *lookupEntityResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	domain, err := o.c.GetDomain(ctx)
	if err != nil {
		return err
	}

	val, err := domain.IdentityLookupEntity(ctx, o.dir)
	if err != nil {
		return err
	}
	if val != nil {
		_, _ = handler.AddValue(val)
	}
	return nil
}

// resolveLookupEntity returns a resolver for looking up an entity.
func (c *Controller) resolveLookupEntity(
	ctx context.Context,
	di directive.Instance,
	dir identity.IdentityLookupEntity,
) (directive.Resolver, error) {
	domainID := dir.IdentityLookupEntityDomainID()
	if c.domainInfo.GetDomainId() != domainID {
		return nil, nil
	}

	// Return resolver.
	return &lookupEntityResolver{c: c, ctx: ctx, dir: dir}, nil
}
