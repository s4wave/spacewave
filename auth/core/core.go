package core

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	auth_derive "github.com/s4wave/spacewave/auth/derive"
	auth_method_password "github.com/s4wave/spacewave/auth/method/password"
	identity_core "github.com/s4wave/spacewave/identity/core"
)

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(auth_derive.NewFactory(b))
	sr.AddFactory(auth_method_password.NewFactory(b))
	identity_core.AddFactories(b, sr)
}
