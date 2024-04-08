package core

import (
	"github.com/aperturerobotics/auth/derive"
	auth_method_triplesec "github.com/aperturerobotics/auth/method/triplesec"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	identity_core "github.com/aperturerobotics/identity/core"
)

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(auth_derive.NewFactory(b))
	sr.AddFactory(auth_method_triplesec.NewFactory(b))
	identity_core.AddFactories(b, sr)
}
