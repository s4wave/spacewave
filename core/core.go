package core

import (
	auth_derive "github.com/aperturerobotics/auth/derive"
	auth_method_password "github.com/aperturerobotics/auth/method/password"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	identity_core "github.com/aperturerobotics/identity/core"
)

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(auth_derive.NewFactory(b))
	sr.AddFactory(auth_method_password.NewFactory(b))
	identity_core.AddFactories(b, sr)
}
