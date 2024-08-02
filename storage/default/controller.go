package storage_default

import (
	storage_controller "github.com/aperturerobotics/bldr/storage/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
)

// StorageID is the storage id usually used for the default storage.
const StorageID = "default"

// ControllerID is the controller identifier.
const ControllerID = "bldr/storage/default"

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// Controller is the storage/default controller.
type Controller = storage_controller.StorageController

// NewController constructs the storage/default controller.
func NewController(storageId string, b bus.Bus, rootDir string) *storage_controller.StorageController {
	storages := BuildStorage(b, rootDir)
	return storage_controller.BuildStorageController(
		storageId,
		storages,
		controller.NewInfo(ControllerID, Version, "default bldr storage"),
	)
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
