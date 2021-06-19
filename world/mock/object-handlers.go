package world_mock

import "github.com/aperturerobotics/hydra/world"

// GetMockObjectOpHandlers builds the set of mock object op handlers.
func GetMockObjectOpHandlers() []world.ApplyObjectOpFunc {
	return []world.ApplyObjectOpFunc{
		ApplyMockObjectOp,
	}
}
