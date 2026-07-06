package objecttypes

import (
	"context"
	"testing"

	forge_dashboard "github.com/s4wave/spacewave/core/forge/dashboard"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_git_world "github.com/s4wave/spacewave/sdk/git/world"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
)

func requireObjectType(t *testing.T, typeID string) {
	t.Helper()

	got, err := LookupObjectType(context.Background(), typeID)
	if err != nil {
		t.Fatalf("LookupObjectType(%s): %v", typeID, err)
	}
	if got == nil {
		t.Fatalf("LookupObjectType(%s) returned nil", typeID)
	}
	if got.GetObjectTypeID() != typeID {
		t.Fatalf("object type id = %q, want %q", got.GetObjectTypeID(), typeID)
	}
}

func TestLookupGitRepoObjectType(t *testing.T) {
	got, err := LookupObjectType(context.Background(), s4wave_git_world.GitRepoTypeID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected Git repository ObjectType")
	}
	if got.GetObjectTypeID() != s4wave_git_world.GitRepoTypeID {
		t.Fatalf("object type id = %q, want %q", got.GetObjectTypeID(), s4wave_git_world.GitRepoTypeID)
	}
}

func TestLookupGitWorktreeObjectType(t *testing.T) {
	got, err := LookupObjectType(context.Background(), s4wave_git_world.GitWorktreeTypeID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected Git worktree ObjectType")
	}
	if got.GetObjectTypeID() != s4wave_git_world.GitWorktreeTypeID {
		t.Fatalf("object type id = %q, want %q", got.GetObjectTypeID(), s4wave_git_world.GitWorktreeTypeID)
	}
}

func TestLookupComputersDashboardObjectType(t *testing.T) {
	got, err := LookupObjectType(context.Background(), s4wave_device.ComputersDashboardTypeID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected Computers dashboard ObjectType")
	}
	if got.GetObjectTypeID() != s4wave_device.ComputersDashboardTypeID {
		t.Fatalf("object type id = %q, want %q", got.GetObjectTypeID(), s4wave_device.ComputersDashboardTypeID)
	}
}

func TestLookupTerminalObjectType(t *testing.T) {
	got, err := LookupObjectType(context.Background(), s4wave_terminal.TerminalTypeID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected Terminal ObjectType")
	}
	if got.GetObjectTypeID() != s4wave_terminal.TerminalTypeID {
		t.Fatalf("object type id = %q, want %q", got.GetObjectTypeID(), s4wave_terminal.TerminalTypeID)
	}
}

func TestLookupSshHostObjectType(t *testing.T) {
	got, err := LookupObjectType(context.Background(), s4wave_sshhost.SshHostTypeID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected SSH Host ObjectType")
	}
	if got.GetObjectTypeID() != s4wave_sshhost.SshHostTypeID {
		t.Fatalf("object type id = %q, want %q", got.GetObjectTypeID(), s4wave_sshhost.SshHostTypeID)
	}
}

func TestLookupForgeObjectTypes(t *testing.T) {
	for _, typeID := range []string{
		forge_cluster.ClusterTypeID,
		forge_job.JobTypeID,
		forge_task.TaskTypeID,
		forge_pass.PassTypeID,
		forge_execution.ExecutionTypeID,
		forge_worker.WorkerTypeID,
		forge_dashboard.ForgeDashboardTypeID,
	} {
		requireObjectType(t, typeID)
	}
}
