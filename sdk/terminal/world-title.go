package s4wave_terminal

import "github.com/s4wave/spacewave/db/world"

// WorldTitle returns the name, which object browsers show for the object.
func (t *Terminal) WorldTitle() string {
	return t.GetName()
}

// _ is a type assertion
var _ world.TitledBlock = (*Terminal)(nil)
