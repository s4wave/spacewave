//go:build !js

package spacewave_cli

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
)

// socketDesktop supplies a shell lifetime to the real web-plugin RPC controller.
type socketDesktop struct {
	// mtx guards generation, presence, and opens.
	mtx sync.Mutex
	// generation identifies the current shell.
	generation uint64
	// presence holds the current shell state and terminal result.
	presence *ccontainer.CContainer[*bldr_web_plugin.WatchDesktopPresenceResponse]
	// opens counts created shells, excluding focus requests.
	opens int
	// entered reports forwarded open requests before the test releases them.
	entered chan string
	// gate holds concurrent requests at the same shell generation.
	gate <-chan struct{}
	// endOnOpen ends the shell before its presence watch can acknowledge it.
	endOnOpen bool
}

// OpenOrFocusMainWindow acknowledges the current shell after the test gate opens.
func (d *socketDesktop) OpenOrFocusMainWindow(ctx context.Context, req *bldr_web_plugin.OpenOrFocusDesktopRequest) (uint64, error) {
	// Start one generation for concurrent requests.
	d.mtx.Lock()
	if d.presence == nil || d.presence.GetValue().GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED {
		d.generation++
		d.presence = ccontainer.NewCContainerVT(&bldr_web_plugin.WatchDesktopPresenceResponse{
			State: bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE,
		})
		d.opens++
	}
	generation := d.generation
	d.mtx.Unlock()

	// Hold each acknowledgement until both socket requests reached the plugin.
	d.entered <- req.GetRoute()
	select {
	case <-d.gate:
		if d.endOnOpen {
			d.closeShell("")
		}
		return generation, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// DesktopPresence returns the completion event for the requested shell.
func (d *socketDesktop) DesktopPresence(generation uint64) *ccontainer.CContainer[*bldr_web_plugin.WatchDesktopPresenceResponse] {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	if generation == d.generation {
		return d.presence
	}
	return nil
}

// HandleDirective supplies the same desktop controller to each plugin RPC.
func (d *socketDesktop) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	if _, ok := inst.GetDirective().(bldr_web_plugin.LookupDesktop); !ok {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]bldr_web_plugin.Desktop{d}), nil)
}

// closeShell ends one generation before the next open creates another.
func (d *socketDesktop) closeShell(failure string) {
	d.mtx.Lock()
	d.presence.SetValue(&bldr_web_plugin.WatchDesktopPresenceResponse{
		State: bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED,
		Error: failure,
	})
	d.mtx.Unlock()
}

// _ is a type assertion.
var _ bldr_web_plugin.Desktop = (*socketDesktop)(nil)
