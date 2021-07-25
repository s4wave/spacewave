package browser_storage

import (
	"github.com/aperturerobotics/bldr/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
)

// storageMethodCtor constructs a storage method.
//
// if sr is set, adds the factories to the resolver.
type storageMethodCtor func(b bus.Bus, sr *static.Resolver) []runtime.Storage

// storageMethods is the list of available storage methods.
var storageMethods []storageMethodCtor

// BuildStorage builds all available storage methods.
//
// if sr is set, adds the factories to the resolver.
func BuildStorage(b bus.Bus, sr *static.Resolver) []runtime.Storage {
	r := make([]runtime.Storage, 0, len(storageMethods))
	for _, ctor := range storageMethods {
		r = append(r, ctor(b, sr)...)
	}
	return r
}
