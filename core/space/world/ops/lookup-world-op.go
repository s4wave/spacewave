//go:build !tinygo && !goscript

package space_world_ops

import (
	"context"

	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	identity_world "github.com/s4wave/spacewave/identity/world"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// LookupWorldOp looks up built-in space world operation types.
func LookupWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		git_world.LookupGitOp,
		identity_world.LookupOp,
		lookupCoreWorldOp,
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
