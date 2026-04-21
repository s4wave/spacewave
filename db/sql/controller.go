package sql

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
)

// Controller is a common implementation of a SQL engine controller.
type Controller struct {
	info        *controller.Info
	dbID        string
	sqlStoreCtr *ccontainer.CContainer[*SqlStore]
	execute     func(ctx context.Context, ctr *ccontainer.CContainer[*SqlStore]) error
}

// NewController constructs a common SQL engine controller.
func NewController(
	info *controller.Info,
	dbID string,
	execute func(ctx context.Context, ctr *ccontainer.CContainer[*SqlStore]) error,
) *Controller {
	return &Controller{
		info:        info,
		dbID:        dbID,
		sqlStoreCtr: ccontainer.NewCContainer[*SqlStore](nil),
		execute:     execute,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// GetSqlStore waits for the store to be built.
func (c *Controller) GetSqlStore(ctx context.Context) (SqlStore, error) {
	val, err := c.sqlStoreCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return *val, nil
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	if c.execute != nil {
		return c.execute(ctx, c.sqlStoreCtr)
	}
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case LookupSqlStore:
		if c.dbID != "" && c.dbID == d.LookupSqlStoreId() {
			return directive.R(directive.NewGetterResolver(c.GetSqlStore), nil)
		}
	}
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
