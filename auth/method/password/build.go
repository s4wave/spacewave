package auth_method_password

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	auth_method "github.com/s4wave/spacewave/auth/method"
	"github.com/s4wave/spacewave/net/crypto"
)

// ExBuildParametersWithUsernamePassword resolves the runtime password method
// and derives trusted default parameters through its shared memory budget.
func ExBuildParametersWithUsernamePassword(ctx context.Context, b bus.Bus, username string, password []byte) (*Parameters, crypto.PrivKey, error) {
	method, release, err := ExAcquirePasswordMethod(ctx, b)
	if err != nil {
		return nil, nil, err
	}
	defer release()
	return method.BuildParametersWithUsernamePassword(ctx, username, password)
}

// ExAcquirePasswordMethod loads the factory-published runtime method and keeps
// its controller active until release is called.
func ExAcquirePasswordMethod(ctx context.Context, b bus.Bus) (*PasswordMethod, func(), error) {
	_, controllerRef, err := b.AddDirective(resolver.NewLoadControllerWithConfig(&Config{}), nil)
	if err != nil {
		return nil, nil, err
	}
	method, methodRef, err := lookup(ctx, b)
	if err != nil {
		controllerRef.Release()
		return nil, nil, err
	}
	return method, func() {
		methodRef.Release()
		controllerRef.Release()
	}, nil
}

func lookup(ctx context.Context, b bus.Bus) (*PasswordMethod, directive.Reference, error) {
	method, ref, err := auth_method.ExAuthLookupMethodRef(ctx, b, MethodID, false)
	if err != nil {
		return nil, nil, err
	}
	passwordMethod, ok := method.(*PasswordMethod)
	if !ok || passwordMethod == nil {
		if ref != nil {
			ref.Release()
		}
		return nil, nil, errors.New("password auth method has invalid type")
	}
	return passwordMethod, ref, nil
}
