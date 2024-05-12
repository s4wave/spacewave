package browser_storage

import (
	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
)

// storageMethodCtor constructs a storage method.
//
// if sr is set, adds the factories to the resolver.
type storageMethodCtor func(b bus.Bus, prefix string) []storage.Storage

// storageMethods is the list of available storage methods.
var storageMethods []storageMethodCtor

// BuildStorage builds all available storage methods.
//
// prefix is used as the IndexedDB prefix in the browser
func BuildStorage(b bus.Bus, prefix string) []storage.Storage {
	r := make([]storage.Storage, 0, len(storageMethods))
	for _, ctor := range storageMethods {
		r = append(r, ctor(b, prefix)...)
	}
	return r
}
