package sql

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
)

// Controller is a common implementation of a SQL engine controller.
type Controller struct {
	info     *controller.Info
	dbID     string
	sqlDbCtr *ccontainer.CContainer[*SqlDB]
	execute  func(ctx context.Context, ctr *ccontainer.CContainer[*SqlDB]) error
}

// NewController constructs a common SQL engine controller.
func NewController(
	info *controller.Info,
	dbID string,
	execute func(ctx context.Context, ctr *ccontainer.CContainer[*SqlDB]) error,
) *Controller {
	return &Controller{
		info:     info,
		dbID:     dbID,
		sqlDbCtr: ccontainer.NewCContainer[*SqlDB](nil),
		execute:  execute,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// GetSqlDB waits for the database to be built.
func (c *Controller) GetSqlDB(ctx context.Context) (SqlDB, error) {
	val, err := c.sqlDbCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return *val, nil
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	if c.execute != nil {
		return c.execute(ctx, c.sqlDbCtr)
	}
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case LookupSqlDB:
		if c.dbID != "" && c.dbID == d.LookupSqlDBId() {
			return directive.R(directive.NewGetterResolver(c.GetSqlDB), nil)
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
