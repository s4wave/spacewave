//go:build !tinygo

package optypes

import (
	"context"

	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

// lookupCoreWorldOp resolves operations shared by native and GoScript Spaces.
func lookupCoreWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		manifest_world.LookupOp,
		s4wave_kv_world.LookupKvSetRootOp,
		s4wave_sshhost.LookupCreateSshHostOp,
		s4wave_wizard.LookupCreateWizardObjectOp,
	}).LookupOp(ctx, opTypeID)
}
