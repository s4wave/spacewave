package identity_domain_client

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/identity"
)

// lookupEntityResolver resolves select entity directives
type lookupEntityResolver struct {
	c   *Client
	ctx context.Context
	dir identity.IdentityLookupEntity
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *lookupEntityResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	domainID := o.dir.IdentityLookupEntityDomainID()
	entityID := o.dir.IdentityLookupEntityID()

	// Lookup the entity
	le := o.c.le.
		WithField("entity-id", entityID).
		WithField("domain-id", domainID)
	le.Info("looking up entity")

	p, pRef, err := o.c.LookupPeer(ctx)
	if err != nil {
		return err
	}
	defer pRef.Release()

	ent, err := o.c.LookupEntity(ctx, p.GetPrivKey(), entityID, domainID)

	handler.AddValue(identity.NewIdentityLookupEntityValue(
		err,
		ent == nil && err == nil,
		ent,
	))
	return nil
}

// resolveLookupEntity returns a resolver for looking up an entity.
func (c *Client) resolveLookupEntity(
	ctx context.Context,
	di directive.Instance,
	dir identity.IdentityLookupEntity,
) (directive.Resolver, error) {
	domainID := dir.IdentityLookupEntityDomainID()
	if !c.DomainIdMatches(domainID) {
		return nil, nil
	}

	// Return resolver.
	return &lookupEntityResolver{c: c, ctx: ctx, dir: dir}, nil
}
