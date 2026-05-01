package objecttypes

import (
	"context"

	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	forge_dashboard "github.com/s4wave/spacewave/core/forge/dashboard"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_canvas_world "github.com/s4wave/spacewave/sdk/canvas/world"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	spacewave_chat_world "github.com/s4wave/spacewave/sdk/chat/world"
	s4wave_forge_world "github.com/s4wave/spacewave/sdk/forge/world"
	s4wave_git_world "github.com/s4wave/spacewave/sdk/git/world"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_org_world "github.com/s4wave/spacewave/sdk/org/world"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	s4wave_vm_world "github.com/s4wave/spacewave/sdk/vm/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

// LookupObjectType looks up an object type by ID.
// Returns nil if not found.
func LookupObjectType(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
	switch typeID {
	case s4wave_layout_world.ObjectLayoutTypeID:
		return s4wave_layout_world.ObjectLayoutType, nil
	case s4wave_unixfs_world.UnixFSTypeID:
		return s4wave_unixfs_world.UnixFSType, nil
	case s4wave_git_world.GitRepoTypeID:
		return s4wave_git_world.GitRepoType, nil
	case s4wave_canvas_world.CanvasTypeID:
		return s4wave_canvas_world.CanvasType, nil
	case s4wave_git_world.GitWorktreeTypeID:
		return s4wave_git_world.GitWorktreeType, nil
	case forge_cluster.ClusterTypeID:
		return s4wave_forge_world.ClusterType, nil
	case forge_job.JobTypeID:
		return s4wave_forge_world.JobType, nil
	case forge_task.TaskTypeID:
		return s4wave_forge_world.TaskType, nil
	case forge_pass.PassTypeID:
		return s4wave_forge_world.PassType, nil
	case forge_execution.ExecutionTypeID:
		return s4wave_forge_world.ExecutionType, nil
	case forge_worker.WorkerTypeID:
		return s4wave_forge_world.WorkerType, nil
	case forge_dashboard.ForgeDashboardTypeID:
		return s4wave_forge_world.DashboardType, nil
	case spacewave_chat.ChatChannelTypeID:
		return spacewave_chat_world.ChatChannelType, nil
	case spacewave_chat.ChatMessageTypeID:
		return spacewave_chat_world.ChatMessageType, nil
	case s4wave_vm.VmV86TypeID:
		return s4wave_vm_world.VmV86Type, nil
	case s4wave_vm.V86ImageTypeID:
		return s4wave_vm_world.V86ImageType, nil
	case s4wave_org.OrganizationTypeID:
		return s4wave_org_world.OrganizationType, nil
	case bldr_manifest_world.ManifestTypeID:
		return objecttype.NewObjectType(bldr_manifest_world.ManifestTypeID, s4wave_forge_world.ForgeReadOnlyFactory), nil
	default:
		return s4wave_wizard.LookupWizardObjectType(ctx, typeID)
	}
}
