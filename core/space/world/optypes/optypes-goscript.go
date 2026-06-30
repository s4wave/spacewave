//go:build goscript

package optypes

import (
	"context"

	s4wave_git "github.com/s4wave/spacewave/core/git"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

// LookupWorldOp looks up the GoScript-supported world operation types.
func LookupWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		unixfs_world.LookupFsOp,
		lookupLocalGitOp,
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
		s4wave_kv_world.LookupKvSetRootOp,
		// TODO: move SQL operations into a separate spacewave-sql plugin.
		// SQL stays out of spacewave-core so go-mysql-server is not in the core bundle.
		// When wiring that plugin, pass the sql_lite build tag so go-mysql-server
		// omits heavy features like collation maps.
		spacewave_chat.LookupInitChatDemoOp,
		spacewave_chat.LookupCreateChatChannelOp,
		s4wave_device.LookupCreateComputersDashboardOp,
		s4wave_sshhost.LookupCreateSshHostOp,
		s4wave_terminal.LookupCreateTerminalOp,
		s4wave_git.LookupCreateGitRepoWizardOp,
		s4wave_wizard.LookupCreateWizardObjectOp,
	}).LookupOp(ctx, opTypeID)
}

func lookupLocalGitOp(_ context.Context, opTypeID string) (world.Operation, error) {
	switch opTypeID {
	case git_world.GitInitOpId:
		return &git_world.GitInitOp{}, nil
	case git_world.GitCreateWorktreeOpId:
		return &git_world.GitCreateWorktreeOp{}, nil
	}
	return nil, nil
}
