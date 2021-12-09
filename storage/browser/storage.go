package browser_storage

import (
	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
)

// storageMethodCtor constructs a storage method.
//
// if sr is set, adds the factories to the resolver.
type storageMethodCtor func(b bus.Bus) []storage.Storage

// storageMethods is the list of available storage methods.
var storageMethods []storageMethodCtor

// BuildStorage builds all available storage methods.
func BuildStorage(b bus.Bus) []storage.Storage {
	r := make([]storage.Storage, 0, len(storageMethods))
	for _, ctor := range storageMethods {
		r = append(r, ctor(b)...)
	}
	return r
}
