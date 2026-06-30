package objecttypes

import (
	"context"
	"testing"

	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_git_world "github.com/s4wave/spacewave/sdk/git/world"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
)

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

func TestLookupOmitsSqlObjectTypesFromCore(t *testing.T) {
	for _, typeID := range []string{
		"sql/db",
		"sql/query",
		"sql/query-result",
		"sql/schema",
		"sql/table-view",
		"sql/workbench",
	} {
		got, err := LookupObjectType(context.Background(), typeID)
		if err != nil {
			t.Fatalf("LookupObjectType(%s): %v", typeID, err)
		}
		if got != nil {
			t.Fatalf("expected SQL ObjectType %s to stay out of spacewave-core, got %q", typeID, got.GetObjectTypeID())
		}
	}
}
