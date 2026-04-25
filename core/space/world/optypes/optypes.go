package optypes

import (
	"context"

	forge_dashboard "github.com/s4wave/spacewave/core/forge/dashboard"
	forge_job_ops "github.com/s4wave/spacewave/core/forge/job"
	forge_task_ops "github.com/s4wave/spacewave/core/forge/task"
	s4wave_git "github.com/s4wave/spacewave/core/git"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_world "github.com/s4wave/spacewave/forge/world"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

// LookupWorldOp looks up the available world operation types.
func LookupWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		unixfs_world.LookupFsOp,
		git_world.LookupGitOp,
		space_world_ops.LookupSetSpaceSettingsOp,
		space_world_ops.LookupInitUnixFSOp,
		space_world_ops.LookupInitObjectLayoutOp,
		space_world_ops.LookupInitCanvasDemoOp,
		space_world_ops.LookupCanvasInitOp,
		space_world_ops.LookupCanvasAddNodeOp,
		space_world_ops.LookupCanvasRemoveNodeOp,
		space_world_ops.LookupCanvasSetNodeOp,
		space_world_ops.LookupCanvasAddEdgeOp,
		space_world_ops.LookupCanvasRemoveEdgeOp,
		spacewave_chat.LookupInitChatDemoOp,
		spacewave_chat.LookupCreateChatChannelOp,
		forge_world.LookupWorldOp,
		forge_dashboard.LookupCreateForgeDashboardOp,
		forge_dashboard.LookupLinkForgeDashboardOp,
		forge_dashboard.LookupInitForgeQuickstartOp,
		s4wave_vm.LookupCreateVmV86Op,
		s4wave_vm.LookupSetV86ConfigOp,
		s4wave_vm.LookupSetV86StateOp,
		s4wave_vm.LookupCreateVmImageOp,
		s4wave_vm.LookupSetVmImageMetadataOp,
		s4wave_org.LookupInitOrganizationOp,
		s4wave_org.LookupUpdateOrgOp,
		s4wave_org.LookupDeleteOrganizationOp,
		forge_job_ops.LookupForgeJobCreateOp,
		forge_task_ops.LookupForgeTaskCreateOp,
		s4wave_git.LookupCreateGitRepoWizardOp,
		s4wave_wizard.LookupCreateWizardObjectOp,
	}).LookupOp(ctx, opTypeID)
}
