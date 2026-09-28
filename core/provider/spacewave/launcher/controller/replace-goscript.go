//go:build goscript

package spacewave_launcher_controller

import (
	"context"

	"github.com/pkg/errors"
)

// prepareAppUpdate is not supported in browser environments.
func (c *Controller) prepareAppUpdate(ctx context.Context) (string, error) {
	return "", errors.New("app update not supported in browser")
}
