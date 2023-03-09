package bldr_project_controller

import (
	"context"

	"github.com/pkg/errors"
)

// BuildTargets compiles the given build target(s)
//
// If the targets list is empty, builds all targets.
func (c *Controller) BuildTargets(ctx context.Context, targets []string) error {
	// TODO
	return errors.New("TODO project controller build targets")
}
