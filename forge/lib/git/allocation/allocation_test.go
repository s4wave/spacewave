package forge_lib_git_allocation

import (
	"context"
	"testing"

	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestCreateOrReuseAllocationLinksForgeAndGitProvenance(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	le := logrus.NewEntry(log)

	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	args := CreateArgs{
		ExecutionObjectKey: "forge/pass/1/execution/peer-a",
		PassObjectKey:      "forge/pass/1",
		RepoObjectKey:      "repo/main",
		WorktreeObjectKey:  "repo/main/worktree/exec-a",
		BaseCommitHash:     "1111111111111111111111111111111111111111",
		BranchRef:          "refs/heads/agent/exec-a",
		PathFamily:         "repos/spacewave",
		EvidenceObjectKey:  "evidence/allocation/1",
		StaleBaseState:     "current",
	}
	for _, objKey := range []string{
		args.ExecutionObjectKey,
		args.PassObjectKey,
		args.RepoObjectKey,
		args.WorktreeObjectKey,
	} {
		if _, err := ws.CreateObject(ctx, objKey, nil); err != nil {
			t.Fatal(err)
		}
	}
	alloc, objKey, rootRef, err := CreateOrReuse(ctx, ws, args)
	if err != nil {
		t.Fatal(err)
	}
	if objKey == "" || rootRef.GetRootRef().GetEmpty() {
		t.Fatalf("expected allocation object key and root ref, got key=%q ref=%+v", objKey, rootRef)
	}
	if alloc.GetStatus() != "allocated" ||
		alloc.GetCollisionState() != "none" ||
		alloc.GetStaleBaseState() != args.StaleBaseState ||
		alloc.GetCleanupState() != "active" ||
		alloc.GetEvidenceObjectKey() != args.EvidenceObjectKey {
		t.Fatalf("unexpected allocation: %+v", alloc)
	}
	if alloc.GetTimestamp() == nil {
		t.Fatal("expected omitted timestamp to be defaulted")
	}

	reused, reusedKey, reusedRef, err := CreateOrReuse(ctx, ws, args)
	if err != nil {
		t.Fatal(err)
	}
	if reusedKey != objKey || !reusedRef.EqualsRef(rootRef) || !reused.sameAllocation(alloc) {
		t.Fatalf("expected idempotent reuse, key=%q/%q ref=%+v/%+v alloc=%+v/%+v", reusedKey, objKey, reusedRef, rootRef, reused, alloc)
	}

	execAllocKeys, err := ListExecutionAllocations(ctx, ws, args.ExecutionObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(execAllocKeys) != 1 || execAllocKeys[0] != objKey {
		t.Fatalf("execution allocations: %+v", execAllocKeys)
	}
	passAllocKeys, err := ListPassAllocations(ctx, ws, args.PassObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(passAllocKeys) != 1 || passAllocKeys[0] != objKey {
		t.Fatalf("pass allocations: %+v", passAllocKeys)
	}
	repoKeys, err := ListAllocationRepos(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(repoKeys) != 1 || repoKeys[0] != args.RepoObjectKey {
		t.Fatalf("allocation repos: %+v", repoKeys)
	}
	worktreeKeys, err := ListAllocationWorktrees(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(worktreeKeys) != 1 || worktreeKeys[0] != args.WorktreeObjectKey {
		t.Fatalf("allocation worktrees: %+v", worktreeKeys)
	}

	collisionArgs := args
	collisionArgs.WorktreeObjectKey = "repo/main/worktree/other"
	if _, _, _, err := CreateOrReuse(ctx, ws, collisionArgs); err == nil {
		t.Fatal("expected collision for same allocation key with different worktree")
	}
}
