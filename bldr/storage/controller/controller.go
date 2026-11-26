package storage_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/bldr/storage"
)

// StorageController is the bldr storage controller.
type StorageController struct {
	// storageId is the id to respond to on the bus
	storageId string
	// storage is the storage
	storage []storage.Storage
	// info is the controller info
	info *controller.Info
}

// BuildStorageController builds a new storage controller.
func BuildStorageController(storageId string, storage []storage.Storage, info *controller.Info) *StorageController {
	return &StorageController{
		storageId: storageId,
		storage:   storage,
		info:      info,
	}
}

// RegisterFactories adds the configured storage factories in storage order.
// It reports whether the controller has any storage methods.
func (c *StorageController) RegisterFactories(b bus.Bus, sr *static.Resolver) bool {
	// Install each method on the process bus.
	for _, method := range c.storage {
		method.AddFactories(b, sr)
	}

	// Let the process builder enforce its storage requirement.
	return len(c.storage) != 0
}

// GetControllerInfo returns information about the controller.
func (c *StorageController) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// Execute executes the controller goroutine.
func (c *StorageController) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns resolver(s). If not, returns nil.
// It is safe to add a reference to the directive during this call.
// The passed context is canceled when the directive instance expires.
// NOTE: the passed context is not canceled when the handler is removed.
func (c *StorageController) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case storage.LookupStorage:
		if dStorageID := d.LookupStorageId(); dStorageID == "" || dStorageID == c.storageId {
			return directive.R(directive.NewValueResolver(c.storage), nil)
		}
	}
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *StorageController) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*StorageController)(nil)
