//go:build js

package spacewave_launcher_controller

import "github.com/pkg/errors"

// applyUpdate is not supported in browser environments.
func (c *Controller) applyUpdate() error {
	return errors.New("self-update not supported in browser")
}
