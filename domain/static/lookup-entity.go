package identity_domain_static

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/identity"
)

// lookupEntityResolver resolves lookup entity from static list
type lookupEntityResolver struct {
	c   *Controller
	di  directive.Instance
	dir identity.IdentityLookupEntity
}

// checkDomainsList checks the domains list.
func checkDomainsList(domains []string, domainID string) (int, bool) {
	if len(domains) == 0 {
		return -1, true
	}

	for i, d := range domains {
		if domainID == d {
			return i, true
		}
	}

	return -1, false
}

// resolveLookupEntity resolves a lookup entity from network directive.
func (c *Controller) resolveLookupEntity(
	ctx context.Context,
	di directive.Instance,
	dir identity.IdentityLookupEntity,
) (directive.Resolver, error) {
	// Check domains list
	domainID := dir.IdentityLookupEntityDomainID()
	if domains := c.conf.GetDomains(); len(domains) != 0 {
		_, found := checkDomainsList(domains, domainID)
		if !found {
			return nil, nil
		}
	}

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

	entity, err := r.c.LookupEntity(domainID, entityID)
	notFound := err == nil && entity == nil

	if err == context.Canceled {
		return nil
	}

	if notFound && r.c.conf.GetSilentNotFound() {
		return nil
	}

	_, _ = handler.AddValue(identity.NewIdentityLookupEntityValue(
		err, notFound, entity,
	))
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupEntityResolver)(nil))
