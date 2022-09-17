package bldr_cli_dist

import "github.com/aperturerobotics/hydra/world"

// BldrDist is the distribution Hydra World.
type BldrDist struct {
	// worldEngine is the world engine handle.
	worldEngine world.Engine
}

// NewBldrDist constructs the bldr distribution world.
func NewBldrDist(eng world.Engine) *BldrDist {
	return &BldrDist{
		worldEngine: eng,
	}
}
