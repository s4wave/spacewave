package forge_dashboard

import "github.com/s4wave/spacewave/db/world"

// WorldTitle returns the name, which object browsers show for the object.
func (d *ForgeDashboard) WorldTitle() string {
	return d.GetName()
}

// _ is a type assertion
var _ world.TitledBlock = (*ForgeDashboard)(nil)
