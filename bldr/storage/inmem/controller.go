package storage_inmem

import (
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/bldr/storage"
	storage_controller "github.com/s4wave/spacewave/bldr/storage/controller"
)

// ControllerID is the controller identifier.
const ControllerID = "bldr/storage/inmem"

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// Controller is the storage controller.
type Controller = storage_controller.StorageController

// NewController constructs the storage controller.
func NewController(storageId string) *storage_controller.StorageController {
	storages := []storage.Storage{NewInmemStorage()}
	return storage_controller.BuildStorageController(
		storageId,
		storages,
		controller.NewInfo(ControllerID, Version, "default bldr storage"),
	)
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
