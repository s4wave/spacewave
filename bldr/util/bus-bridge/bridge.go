package bus_bridge

// TODO: move this code to controllerbus/bus/bridge and replace old impl.

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/sirupsen/logrus"
)

// BusBridgeFilter filters or transforms an incoming directive.
//
// The directives returned will be forwarded to the host bus.
// If no directives are returned the directive is ignored.
type BusBridgeFilter func(ctx context.Context, di directive.Instance) ([]directive.Directive, error)

// BusBridge accepts HandleDirective on one bus and forwards to another bus.
// The directives can be filtered or transformed by an attached function.
// If the filter is not set, the controller passes through all directives.
type BusBridge struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// info contains the controller info
	info *controller.Info
	// filter is the filter callback
	filter BusBridgeFilter
}

// NewBusBridge constructs a new controller.
func NewBusBridge(
	le *logrus.Entry,
	bus bus.Bus,
	info *controller.Info,
	filter BusBridgeFilter,
) *BusBridge {
	return &BusBridge{
		le:     le,
		bus:    bus,
		info:   info,
		filter: filter,
	}
}

// GetControllerInfo returns information about the controller.
func (b *BusBridge) GetControllerInfo() *controller.Info {
	return b.info.Clone()
}

// Execute executes the controller goroutine.
func (b *BusBridge) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns resolver(s). If not, returns nil.
// It is safe to add a reference to the directive during this call.
// The passed context is canceled when the directive instance expires.
// NOTE: the passed context is not canceled when the handler is removed.
func (b *BusBridge) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	var dirs []directive.Directive
	if b.filter != nil {
		var err error
		dirs, err = b.filter(ctx, di)
		if err != nil {
			return nil, err
		}
	} else {
		dirs = []directive.Directive{di.GetDirective()}
	}

	res := make([]directive.Resolver, len(dirs))
	for i, dir := range dirs {
		res[i] = NewBusBridgeResolver(b.bus, dir)
	}
	return res, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (b *BusBridge) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*BusBridge)(nil)
