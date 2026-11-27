//go:build !js

package layout_testbed

import (
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// Setup holds the test setup state for layout tests.
type Setup struct {
	// Testbed is the layout testbed.
	Testbed *Testbed
	// Engine is the world engine.
	Engine *s4wave_world.Engine
	// LayoutResourceID is the resource ID of the created layout.
	LayoutResourceID uint32
	// layoutResourceRef keeps LayoutResourceID adopted for the setup
	// lifetime.
	layoutResourceRef resource_client.ResourceRef
}

// Release releases the setup resources.
func (s *Setup) Release() {
	if s.layoutResourceRef != nil {
		s.layoutResourceRef.Release()
	}
	if s.Engine != nil {
		s.Engine.Release()
	}
}
