//go:build js

package resource_listener

import "context"

// Execute is a no-op on the js platform.
// Unix sockets are not available in the browser.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}
