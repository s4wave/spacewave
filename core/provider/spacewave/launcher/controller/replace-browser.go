//go:build js || goscript

package spacewave_launcher_controller

import (
	"context"

	"github.com/pkg/errors"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// prepareAppUpdate is not supported in browser environments.
func (c *Controller) prepareAppUpdate(ctx context.Context) (string, error) {
	return "", errors.New("app update not supported in browser")
}

// prepareDaemonUpdate is not supported in browser environments.
func (c *Controller) prepareDaemonUpdate(ctx context.Context) (*spacewave_launcher.UpdateState, error) {
	return nil, errors.New("daemon update not supported in browser")
}
