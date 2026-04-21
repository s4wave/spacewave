package auth_method_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	auth_method "github.com/s4wave/spacewave/auth/method"
)

// authLookupMethodResolver resolves AuthLookupMethod directives
type authLookupMethodResolver struct {
	c   *Controller
	di  directive.Instance
	dir auth_method.AuthLookupMethod
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *authLookupMethodResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// if we already resolved the keypair, return.
	if handler.CountValues(false) != 0 {
		return nil
	}

	c := o.c
	methodID := o.dir.AuthLookupMethodID()
	if o.c.methodID != methodID {
		return nil
	}

	method, err := c.GetAuthMethod(ctx)
	if err != nil {
		return err
	}

	_, _ = handler.AddValue(method)
	return nil
}

// resolveAuthLookupMethod returns a resolver for an authentication method.
func (c *Controller) resolveAuthLookupMethod(
	di directive.Instance,
	dir auth_method.AuthLookupMethod,
) (directive.Resolver, error) {
	methodID := dir.AuthLookupMethodID()
	if c.methodID != methodID {
		return nil, nil
	}

	return &authLookupMethodResolver{c: c, di: di, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*authLookupMethodResolver)(nil))
