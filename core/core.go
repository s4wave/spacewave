package core

import (
	challenge_client "github.com/aperturerobotics/auth/challenge/client"
	challenge_server "github.com/aperturerobotics/auth/challenge/server"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
)

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(challenge_client.NewFactory(b))
	sr.AddFactory(challenge_server.NewFactory(b))
}
