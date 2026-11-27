// Package forge_dashboard implements the ForgeDashboard block type and world
// operations.
package forge_dashboard

import (
	"github.com/aperturerobotics/cayley/quad"
)

// PredDashboardForgeRef is the graph predicate for linking a dashboard to a
// Forge entity.
var PredDashboardForgeRef = quad.IRI("dashboard/forge-ref")

// Validate cursorily checks the ForgeDashboard.
func (d *ForgeDashboard) Validate() error {
	return nil
}
