//+build !js

package core

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/hydra/volume/badger"
	"github.com/aperturerobotics/hydra/volume/bolt"
	"github.com/aperturerobotics/hydra/volume/redis"
)

// addNativeFactories adds factories specific to this platform.
func addNativeFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_badger.NewFactory(b))
	sr.AddFactory(volume_bolt.NewFactory(b))
	sr.AddFactory(volume_redis.NewFactory(b))
}
