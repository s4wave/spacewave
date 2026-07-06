//go:build goscript

package space_world_ops

import (
	"context"
	"testing"

	forge_dashboard "github.com/s4wave/spacewave/core/forge/dashboard"
	forge_job_ops "github.com/s4wave/spacewave/core/forge/job"
	forge_task_ops "github.com/s4wave/spacewave/core/forge/task"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_pass_tx "github.com/s4wave/spacewave/forge/pass/tx"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

func requireLookupWorldOp[T any](t *testing.T, opTypeID string) {
	t.Helper()

	op, err := LookupWorldOp(context.Background(), opTypeID)
	if err != nil {
		t.Fatalf("LookupWorldOp(%s): %v", opTypeID, err)
	}
	if _, ok := op.(T); !ok {
		t.Fatalf("LookupWorldOp(%s) = %T, want %T", opTypeID, op, *new(T))
	}
}

func TestGoScriptLookupWorldOpResolvesDeviceCoreOps(t *testing.T) {
	requireLookupWorldOp[*s4wave_device.CreateComputersDashboardOp](t, s4wave_device.CreateComputersDashboardOpId)
}

func TestGoScriptLookupWorldOpResolvesForgeQuickstartOps(t *testing.T) {
	requireLookupWorldOp[*forge_dashboard.CreateForgeDashboardOp](t, forge_dashboard.CreateForgeDashboardOpId)
	requireLookupWorldOp[*forge_dashboard.LinkForgeDashboardOp](t, forge_dashboard.LinkForgeDashboardOpId)
	requireLookupWorldOp[*forge_dashboard.InitForgeQuickstartOp](t, forge_dashboard.InitForgeQuickstartOpId)
	requireLookupWorldOp[*forge_job_ops.ForgeJobCreateOp](t, forge_job_ops.ForgeJobCreateOpId)
	requireLookupWorldOp[*forge_task_ops.ForgeTaskCreateOp](t, forge_task_ops.ForgeTaskCreateOpId)
	requireLookupWorldOp[*forge_cluster.ClusterCreateOp](t, forge_cluster.ClusterCreateOpId)
	requireLookupWorldOp[*forge_cluster.ClusterAssignJobOp](t, forge_cluster.ClusterAssignJobOpId)
	requireLookupWorldOp[*forge_cluster.ClusterAssignWorkerOp](t, forge_cluster.ClusterAssignWorkerOpId)
	requireLookupWorldOp[*forge_cluster.ClusterAssignTaskOp](t, forge_cluster.ClusterAssignTaskOpId)
	requireLookupWorldOp[*forge_cluster.ClusterAssignPeerOp](t, forge_cluster.ClusterAssignPeerOpId)
	requireLookupWorldOp[*forge_worker.WorkerCreateOp](t, forge_worker.WorkerCreateOpId)
	requireLookupWorldOp[*forge_execution_tx.Tx](t, forge_execution_tx.ObjectOperationTypeID)
	requireLookupWorldOp[*forge_pass_tx.Tx](t, forge_pass_tx.WorldOperationTypeID)
}
