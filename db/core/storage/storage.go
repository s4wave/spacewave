// Package storage registers database factories without network controllers.
package storage

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_overlay "github.com/s4wave/spacewave/db/block/store/overlay"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	bucket_setup "github.com/s4wave/spacewave/db/bucket/setup"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
)

// AddFactories registers the shared node, bucket, lookup, and in-memory stores.
func AddFactories(b bus.Bus, resolver *static.Resolver) {
	// Register each storage factory on the resolver.
	resolver.AddFactory(bucket_setup.NewFactory(b))
	resolver.AddFactory(node_controller.NewFactory(b))
	resolver.AddFactory(lookup_concurrent.NewFactory(b))
	resolver.AddFactory(volume_kvtxinmem.NewFactory(b))
	resolver.AddFactory(block_store_inmem.NewFactory(b))
	resolver.AddFactory(block_store_overlay.NewFactory(b))
}
