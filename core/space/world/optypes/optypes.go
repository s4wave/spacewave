//go:build !tinygo && !goscript

package optypes

import (
	"context"

	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
	s4wave_apt "github.com/s4wave/spacewave/sdk/apt"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// LookupWorldOp looks up the available world operation types.
func LookupWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		space_world_ops.LookupWorldOp,
		lookupCoreWorldOp,
		s4wave_apt.LookupAptOp,
		s4wave_vm.LookupCreateVmV86Op,
		s4wave_vm.LookupSetV86ConfigOp,
		s4wave_vm.LookupSetV86StateOp,
		s4wave_vm.LookupCreateV86ImageOp,
		s4wave_vm.LookupSetV86ImageMetadataOp,
		s4wave_org.LookupInitOrganizationOp,
		s4wave_org.LookupUpdateOrgOp,
		s4wave_org.LookupDeleteOrganizationOp,
	}).LookupOp(ctx, opTypeID)
}
