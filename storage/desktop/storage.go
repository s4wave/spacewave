package electron_storage

import (
	"github.com/aperturerobotics/bldr/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
)

// storageMethodCtor constructs a storage method.
type storageMethodCtor func(b bus.Bus) []runtime.Storage

// storageMethods is the list of available storage methods.
var storageMethods []storageMethodCtor

// BuildStorage builds all available storage methods.
func BuildStorage(b bus.Bus) []runtime.Storage {
	r := make([]runtime.Storage, 0, len(storageMethods))
	for _, ctor := range storageMethods {
		r = append(r, ctor(b)...)
	}
	return r
}
