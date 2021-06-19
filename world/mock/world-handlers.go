package world_mock

import "github.com/aperturerobotics/hydra/world"

// GetMockWorldOpHandlers builds the set of mock object op handlers.
func GetMockWorldOpHandlers() []world.ApplyWorldOpFunc {
	return []world.ApplyWorldOpFunc{
		ApplyMockWorldOp,
	}
}
