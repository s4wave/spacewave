package optypes

import (
	"context"
	"runtime"
	"testing"

	s4wave_git "github.com/s4wave/spacewave/core/git"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	git_world "github.com/s4wave/spacewave/db/git/world"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

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
