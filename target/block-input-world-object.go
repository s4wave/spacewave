package forge_target

import "github.com/aperturerobotics/hydra/world"

// Validate validates the input world object.
func (i *InputWorldObject) Validate() error {
	if i.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	return nil
}
