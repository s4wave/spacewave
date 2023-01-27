package identity_domain_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	identity_domain "github.com/aperturerobotics/identity/domain"
)

// lookupIdentityDomainResolver resolves lookup domain directives
type lookupIdentityDomainResolver struct {
	c   *Controller
	ctx context.Context
	dir identity_domain.LookupIdentityDomain
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *lookupIdentityDomainResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	d, err := o.c.GetDomain(ctx)
	if err != nil {
		return err
	}
	di := d.GetDomainInfo()
	var val identity_domain.LookupIdentityDomainValue = di
	handler.AddValue(val)
	return nil
}

// resolveLookupIdentityDomain returns a resolver for looking up a domain.
func (c *Controller) resolveLookupIdentityDomain(
	ctx context.Context,
	di directive.Instance,
	dir identity_domain.LookupIdentityDomain,
) (directive.Resolver, error) {
	lookupID := dir.LookupIdentityDomainId()
	if lookupID != "" && c.domainInfo.GetDomainId() != lookupID {
		return nil, nil
	}

	// Return resolver.
	return &lookupIdentityDomainResolver{c: c, ctx: ctx, dir: dir}, nil
}
