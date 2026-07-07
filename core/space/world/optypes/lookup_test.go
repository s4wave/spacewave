package optypes

import (
	"context"
	"runtime"
	"testing"

	forge_dashboard "github.com/s4wave/spacewave/core/forge/dashboard"
	forge_job_ops "github.com/s4wave/spacewave/core/forge/job"
	forge_task_ops "github.com/s4wave/spacewave/core/forge/task"
	s4wave_git "github.com/s4wave/spacewave/core/git"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_pass_tx "github.com/s4wave/spacewave/forge/pass/tx"
	forge_task_tx "github.com/s4wave/spacewave/forge/task/tx"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

func requireLookupOp[T any](t *testing.T, lookup world.LookupOp, lookupName string, opTypeID string) {
	t.Helper()

	op, err := lookup(context.Background(), opTypeID)
	if err != nil {
		t.Fatalf("%s(%s): %v", lookupName, opTypeID, err)
	}
	if _, ok := op.(T); !ok {
		t.Fatalf("%s(%s) = %T, want %T", lookupName, opTypeID, op, *new(T))
	}
}

func requireLookupWorldAndBuildSpaceOp[T any](t *testing.T, opTypeID string) {
	t.Helper()

	requireLookupOp[T](t, LookupWorldOp, "LookupWorldOp", opTypeID)
	requireLookupOp[T](t, BuildSpaceLookupOp(nil, nil, "space/local/test"), "BuildSpaceLookupOp", opTypeID)
}

func TestBuildSpaceLookupOpResolvesBuiltInWithoutBus(t *testing.T) {
	lookupOp := BuildSpaceLookupOp(nil, nil, "space/local/test")

	op, err := lookupOp(context.Background(), space_world_ops.InitUnixFSOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*space_world_ops.InitUnixFSOp); !ok {
		t.Fatalf("expected InitUnixFSOp, got %T", op)
	}

	op, err = lookupOp(context.Background(), git_world.GitInitOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*git_world.GitInitOp); !ok {
		t.Fatalf("expected GitInitOp, got %T", op)
	}

	op, err = lookupOp(context.Background(), git_world.GitCreateWorktreeOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*git_world.GitCreateWorktreeOp); !ok {
		t.Fatalf("expected GitCreateWorktreeOp, got %T", op)
	}

	op, err = lookupOp(context.Background(), s4wave_git.CreateGitRepoWizardOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*s4wave_git.CreateGitRepoWizardOp); !ok {
		t.Fatalf("expected CreateGitRepoWizardOp, got %T", op)
	}

	op, err = lookupOp(context.Background(), s4wave_wizard.CreateWizardObjectOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*s4wave_wizard.CreateWizardObjectOp); !ok {
		t.Fatalf("expected CreateWizardObjectOp, got %T", op)
	}

	op, err = lookupOp(context.Background(), s4wave_device.CreateComputersDashboardOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*s4wave_device.CreateComputersDashboardOp); !ok {
		t.Fatalf("expected CreateComputersDashboardOp, got %T", op)
	}

	op, err = lookupOp(context.Background(), s4wave_terminal.CreateTerminalOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*s4wave_terminal.CreateTerminalOp); !ok {
		t.Fatalf("expected CreateTerminalOp, got %T", op)
	}

	op, err = lookupOp(context.Background(), spacewave_chat.InitChatDemoOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*spacewave_chat.InitChatDemoOp); !ok {
		t.Fatalf("expected InitChatDemoOp, got %T", op)
	}

	op, err = lookupOp(context.Background(), spacewave_chat.CreateChatChannelOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*spacewave_chat.CreateChatChannelOp); !ok {
		t.Fatalf("expected CreateChatChannelOp, got %T", op)
	}

	op, err = lookupOp(context.Background(), s4wave_sshhost.CreateSshHostOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*s4wave_sshhost.CreateSshHostOp); !ok {
		t.Fatalf("expected CreateSshHostOp, got %T", op)
	}

	op, err = lookupOp(context.Background(), s4wave_kv_world.KvSetRootOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*s4wave_kv_world.KvSetRootOp); !ok {
		t.Fatalf("expected KvSetRootOp, got %T", op)
	}

}

func TestBuildSpaceLookupOpAndLookupWorldOpResolveForgeQuickstartOps(t *testing.T) {
	requireLookupWorldAndBuildSpaceOp[*forge_dashboard.CreateForgeDashboardOp](t, forge_dashboard.CreateForgeDashboardOpId)
	requireLookupWorldAndBuildSpaceOp[*forge_dashboard.LinkForgeDashboardOp](t, forge_dashboard.LinkForgeDashboardOpId)
	requireLookupWorldAndBuildSpaceOp[*forge_dashboard.InitForgeQuickstartOp](t, forge_dashboard.InitForgeQuickstartOpId)
	requireLookupWorldAndBuildSpaceOp[*forge_job_ops.ForgeJobCreateOp](t, forge_job_ops.ForgeJobCreateOpId)
	requireLookupWorldAndBuildSpaceOp[*forge_task_ops.ForgeTaskCreateOp](t, forge_task_ops.ForgeTaskCreateOpId)
	requireLookupWorldAndBuildSpaceOp[*forge_cluster.ClusterCreateOp](t, forge_cluster.ClusterCreateOpId)
	requireLookupWorldAndBuildSpaceOp[*forge_cluster.ClusterAssignJobOp](t, forge_cluster.ClusterAssignJobOpId)
	requireLookupWorldAndBuildSpaceOp[*forge_cluster.ClusterAssignWorkerOp](t, forge_cluster.ClusterAssignWorkerOpId)
	requireLookupWorldAndBuildSpaceOp[*forge_cluster.ClusterAssignTaskOp](t, forge_cluster.ClusterAssignTaskOpId)
	requireLookupWorldAndBuildSpaceOp[*forge_cluster.ClusterAssignPeerOp](t, forge_cluster.ClusterAssignPeerOpId)
	requireLookupWorldAndBuildSpaceOp[*forge_worker.WorkerCreateOp](t, forge_worker.WorkerCreateOpId)
	requireLookupWorldAndBuildSpaceOp[*forge_execution_tx.Tx](t, forge_execution_tx.ObjectOperationTypeID)
	requireLookupWorldAndBuildSpaceOp[*forge_pass_tx.Tx](t, forge_pass_tx.WorldOperationTypeID)
	requireLookupWorldAndBuildSpaceOp[*forge_task_tx.Tx](t, forge_task_tx.WorldOperationTypeID)
}

func TestBuildSpaceLookupOpReturnsNilForUnknownWithoutBus(t *testing.T) {
	lookupOp := BuildSpaceLookupOp(nil, nil, "space/local/test")

	op, err := lookupOp(context.Background(), "space/world/custom-op")
	if err != nil {
		t.Fatal(err)
	}
	if op != nil {
		t.Fatalf("expected nil op, got %T", op)
	}
}

func TestBuildSpaceLookupOpExcludesRemoteGitOpsUnderGoScript(t *testing.T) {
	if runtime.GOOS != "js" {
		t.Skip("GoScript-only lookup boundary")
	}

	lookupOp := BuildSpaceLookupOp(nil, nil, "space/local/test")
	for _, opTypeID := range []string{
		git_world.GitCloneOpId,
		git_world.GitFetchOpId,
	} {
		op, err := lookupOp(context.Background(), opTypeID)
		if err != nil {
			t.Fatal(err)
		}
		if op != nil {
			t.Fatalf("expected %s to remain unavailable under GoScript, got %T", opTypeID, op)
		}
	}
}
