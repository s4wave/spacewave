//go:build !tinygo

package space_world_ops

import (
	"context"

	forge_dashboard "github.com/s4wave/spacewave/core/forge/dashboard"
	forge_job_ops "github.com/s4wave/spacewave/core/forge/job"
	forge_task_ops "github.com/s4wave/spacewave/core/forge/task"
	s4wave_git "github.com/s4wave/spacewave/core/git"
	"github.com/s4wave/spacewave/core/git/opids"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_world "github.com/s4wave/spacewave/forge/world"
	identity_world "github.com/s4wave/spacewave/identity/world"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

func lookupCoreWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		unixfs_world.LookupFsOp,
		identity_world.LookupOp,
		LookupSetSpaceSettingsOp,
		LookupInitUnixFSOp,
		LookupInitObjectLayoutOp,
		LookupInitCanvasDemoOp,
		LookupCanvasInitOp,
		LookupCanvasAddNodeOp,
		LookupCanvasRemoveNodeOp,
		LookupCanvasSetNodeOp,
		LookupCanvasAddEdgeOp,
		LookupCanvasRemoveEdgeOp,
		spacewave_chat.LookupInitChatDemoOp,
		spacewave_chat.LookupCreateChatChannelOp,
		spacewave_chat.LookupSendChatMessageOp,
		spacewave_chat.LookupUpdateChatReadPositionOp,
		s4wave_device.LookupCreateComputersDashboardOp,
		s4wave_terminal.LookupCreateTerminalOp,
		forge_world.LookupWorldOp,
		forge_dashboard.LookupCreateForgeDashboardOp,
		forge_dashboard.LookupLinkForgeDashboardOp,
		forge_dashboard.LookupInitForgeQuickstartOp,
		forge_job_ops.LookupForgeJobCreateOp,
		forge_task_ops.LookupForgeTaskCreateOp,
		s4wave_vm.LookupCreateVmV86Op,
		s4wave_vm.LookupSetV86ConfigOp,
		s4wave_vm.LookupSetV86StateOp,
		s4wave_vm.LookupCreateV86ImageOp,
		s4wave_vm.LookupSetV86ImageMetadataOp,
		lookupGitWizardOp,
	}).LookupOp(ctx, opTypeID)
}

// lookupGitWizardOp avoids loading Git for unrelated world operations.
func lookupGitWizardOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID != opids.CreateRepoWizard {
		return nil, nil
	}
	return s4wave_git.LookupCreateGitRepoWizardOp(ctx, opTypeID)
}
