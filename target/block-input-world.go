package forge_target

import "github.com/aperturerobotics/hydra/world"

// Validate validates the input world object.
func (i *InputWorld) Validate() error {
	if i.GetEngineId() == "" {
		return world.ErrEmptyEngineID
	}
	return nil
}
