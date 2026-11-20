package forge_cluster

import "github.com/s4wave/spacewave/db/world"

// WorldTitle returns the name, which object browsers show for the object.
func (e *Cluster) WorldTitle() string {
	return e.GetName()
}

// _ is a type assertion
var _ world.TitledBlock = (*Cluster)(nil)
