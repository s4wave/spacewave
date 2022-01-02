package auth_challenge_client

import (
	"context"

	auth_challenge "github.com/aperturerobotics/auth/challenge"
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
	domainID := r.dir.IdentityLookupEntityDomainID()

	var found bool
	for _, availDomainID := range r.c.conf.GetDomainIds() {
		if domainID == availDomainID {
			found = true
			break
		}
	}
	if !found {
		return nil
	}

	resCh := make(chan *auth_challenge.EntityLookupFinish, 1)
	ref, refID := r.c.getOrAddLookup(
		domainID,
		entityID,
		func(res *auth_challenge.EntityLookupFinish) {
			select {
			case resCh <- res:
			default:
			}
		},
	)
	defer r.c.releaseLookup(ref, refID)
	r.c.wake()

	// wait for lookup
	var res *auth_challenge.EntityLookupFinish
	select {
	case <-ctx.Done():
		return ctx.Err()
	case res = <-resCh:
	}

	// type assertion
	var resValue identity.IdentityLookupEntityValue = newLookupEntityValue(res)
	_, _ = handler.AddValue(resValue)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupEntityResolver)(nil))
