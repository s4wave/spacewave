//go:build !js

package cli

import (
	"os"
	"time"

	"github.com/aperturerobotics/cli"

	s4wave_debug "github.com/s4wave/spacewave/sdk/debug"
)

// RunWait polls until the debug bridge is ready and responds.
func (a *ClientArgs) RunWait(c *cli.Context) error {
	ctx := c.Context
	w := os.Stderr
	w.WriteString("waiting for debug bridge...")
	for {
		svc, err := a.BuildClient()
		if err == nil {
			_, err = svc.GetPageInfo(ctx, &s4wave_debug.GetPageInfoRequest{})
		}
		if err == nil {
			w.WriteString(" ready\n")
			return nil
		}
		// Reset cached client so next iteration retries the socket.
		a.client = nil
		a.conn = nil
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
