package core

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	identity_domain_client "github.com/aperturerobotics/identity/domain/service/client"
	identity_domain_server "github.com/aperturerobotics/identity/domain/service/server"
	identity_domain_static "github.com/aperturerobotics/identity/domain/static"
)

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(identity_domain_client.NewFactory(b))
	sr.AddFactory(identity_domain_server.NewFactory(b))
	sr.AddFactory(identity_domain_static.NewFactory(b))
}
