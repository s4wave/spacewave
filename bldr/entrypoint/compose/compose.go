// Package compose carries the project-supplied part of a host process.
//
// A project names one Go package as its compose package in the dist and CLI
// compiler configs. That package exports Compose() *compose.Composition, which
// the generated main calls once per process. The composition owns any state its
// factories and commands share, such as process-wide brokers, so Bldr wires
// project controllers without knowing their types.
package compose

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
)

// AddFactoriesFunc constructs controller factories bound to a host bus.
type AddFactoriesFunc = func(b bus.Bus) []controller.Factory

// AddFactories adds the composition's factories to a host bus resolver.
// A nil composition adds nothing.
func (c *Composition) AddFactories(b bus.Bus, sr *static.Resolver) {
	if c == nil {
		return
	}
	for _, addFactories := range c.Factories {
		for _, factory := range addFactories(b) {
			sr.AddFactory(factory)
		}
	}
}
