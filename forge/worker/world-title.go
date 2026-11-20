package forge_worker

import "github.com/s4wave/spacewave/db/world"

// WorldTitle returns the name, which object browsers show for the object.
func (e *Worker) WorldTitle() string {
	return e.GetName()
}

// _ is a type assertion
var _ world.TitledBlock = (*Worker)(nil)
