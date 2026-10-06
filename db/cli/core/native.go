//go:build !js

package hydra_cli_core

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_badger "github.com/s4wave/spacewave/db/volume/badger"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	volume_redis "github.com/s4wave/spacewave/db/volume/redis"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
)

// AddFactories adds the cli storage factories to the resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	// Register the native volume engines.
	sr.AddFactory(volume_badger.NewFactory(b))
	sr.AddFactory(volume_redis.NewFactory(b))
	sr.AddFactory(volume_s4db.NewFactory(b))
	sr.AddFactory(volume_kvtxinmem.NewFactory(b))
}
