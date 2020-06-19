package auth_method_controller

import (
	"context"

	auth_method "github.com/aperturerobotics/auth/method"
	"github.com/aperturerobotics/controllerbus/directive"
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
	c := o.c
	methodID := o.dir.AuthLookupMethodID()
	if o.c.methodID != methodID {
		return nil
	}

	var method auth_method.Method
	select {
	case <-ctx.Done():
		return ctx.Err()
	case method = <-c.methodCh:
		c.methodCh <- method
	}

	// type assertion
	var outp auth_method.AuthLookupMethodValue = method
	_, _ = handler.AddValue(outp)
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
