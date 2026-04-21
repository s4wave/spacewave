//go:build !js

package storage_native

import (
	"github.com/s4wave/spacewave/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
)

// storageMethodCtor constructs a storage method.
type storageMethodCtor func(b bus.Bus, rootDir string) []storage.Storage

// storageMethods is the list of available storage methods.
var storageMethods []storageMethodCtor

// BuildStorage builds all available storage methods.
func BuildStorage(b bus.Bus, rootDir string) []storage.Storage {
	r := make([]storage.Storage, 0, len(storageMethods))
	for _, ctor := range storageMethods {
		r = append(r, ctor(b, rootDir)...)
	}
	return r
}
