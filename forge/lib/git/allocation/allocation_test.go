package forge_lib_git_allocation

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"

	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestCreateOrReuseAllocationLinksForgeAndGitProvenance(t *testing.T) {
	// Create the context and logger for allocation provenance checks.
	ctx := context.Background()
	log := logrus.New()
	le := logrus.NewEntry(log)

	// Start a World testbed backed by the in-memory block testbed.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Configure a Git worktree allocation with Forge provenance.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	args := CreateArgs{
		ExecutionObjectKey: "forge/pass/1/execution/peer-a",
		PassObjectKey:      "forge/pass/1",
		RepoObjectKey:      "repo/main",
		WorktreeObjectKey:  "repo/main/worktree/exec-a",
		WorkdirObjectKey:   "repo/main/worktree/exec-a/workdir",
		BaseCommitHash:     "1111111111111111111111111111111111111111",
		BranchRef:          "refs/heads/agent/exec-a",
		PathFamily:         "repos/spacewave",
		EvidenceObjectKey:  "evidence/allocation/1",
		StaleBaseState:     "current",
	}

	// Create the execution, pass, repo and worktree objects linked by the allocation.
	for _, objKey := range []string{
		args.ExecutionObjectKey,
		args.PassObjectKey,
		args.RepoObjectKey,
		args.WorktreeObjectKey,
	} {
		{
			createdObject, err := ws.CreateObject(ctx, objKey, nil)
			world.ReleaseObjectState(createdObject)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// Create the allocation record in the World.
	alloc, objKey, rootRef, err := CreateOrReuse(ctx, ws, args)
	if err != nil {
		t.Fatal(err)
	}

	// Require allocation defaults, evidence and a durable root reference.
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

	// Require a repeated allocation request to reuse the same record and root.
	reused, reusedKey, reusedRef, err := CreateOrReuse(ctx, ws, args)
	if err != nil {
		t.Fatal(err)
	}
	if reusedKey != objKey || !reusedRef.EqualsRef(rootRef) || !reused.sameAllocation(alloc) {
		t.Fatalf("expected idempotent reuse, key=%q/%q ref=%+v/%+v alloc=%+v/%+v", reusedKey, objKey, reusedRef, rootRef, reused, alloc)
	}

	// Require the execution graph to link to the allocation.
	execAllocKeys, err := ListExecutionAllocations(ctx, ws, args.ExecutionObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(execAllocKeys) != 1 || execAllocKeys[0] != objKey {
		t.Fatalf("execution allocations: %+v", execAllocKeys)
	}

	// Require the pass graph to link to the allocation.
	passAllocKeys, err := ListPassAllocations(ctx, ws, args.PassObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(passAllocKeys) != 1 || passAllocKeys[0] != objKey {
		t.Fatalf("pass allocations: %+v", passAllocKeys)
	}

	// Require the allocation graph to link to its repo.
	repoKeys, err := ListAllocationRepos(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(repoKeys) != 1 || repoKeys[0] != args.RepoObjectKey {
		t.Fatalf("allocation repos: %+v", repoKeys)
	}

	// Require the allocation graph to link to its worktree.
	worktreeKeys, err := ListAllocationWorktrees(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(worktreeKeys) != 1 || worktreeKeys[0] != args.WorktreeObjectKey {
		t.Fatalf("allocation worktrees: %+v", worktreeKeys)
	}

	// Record completed runtime cleanup on the allocation.
	receipt := &forge_runtime.CleanupReceipt{
		ReservationObjectKey: "forge/runtime/reservation/abc",
		ExecutionObjectKey:   args.ExecutionObjectKey,
		RuntimeIdentity:      "container-1",
		Generation:           2,
		RuntimeStopped:       true,
		CapacityReleased:     true,
		Reason:               "cancelled",
	}
	cleaned, cleanedRef, err := RecordCleanup(ctx, ws, objKey, receipt)
	if err != nil {
		t.Fatal(err)
	}

	// Require the allocation cleanup state to reflect complete release.
	if cleaned.GetCleanupState() != "released" || !cleaned.GetCleanup().Complete() {
		t.Fatalf("unexpected cleanup record: %+v state=%q", cleaned.GetCleanup(), cleaned.GetCleanupState())
	}

	// Reload the allocation to verify its persisted cleanup receipt.
	reloaded, err := Lookup(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err)
	}

	// Require the persisted receipt and Workdir identity to match the allocation.
	if reloaded.GetCleanup() == nil ||
		reloaded.GetCleanup().RuntimeIdentity != "container-1" ||
		reloaded.GetCleanup().Generation != 2 ||
		reloaded.GetCleanup().Reason != "cancelled" ||
		reloaded.GetWorkdirObjectKey() != args.WorkdirObjectKey {
		t.Fatalf("cleanup did not persist: %+v", reloaded)
	}
	if !cleanedRef.EqualsRef(mustRootRef(t, ctx, ws, objKey)) {
		t.Fatal("returned ref does not match persisted object root")
	}

	// Require a cleanup receipt without a reason to be rejected.
	partial := &forge_runtime.CleanupReceipt{
		ReservationObjectKey: receipt.ReservationObjectKey,
		ExecutionObjectKey:   args.ExecutionObjectKey,
		Generation:           2,
		CapacityReleased:     true,
	}
	if _, _, err := RecordCleanup(ctx, ws, objKey, partial); err == nil {
		t.Fatal("expected receipt without reason to be rejected")
	}

	// Require an allocation key collision to reject a different worktree.
	collisionArgs := args
	collisionArgs.WorktreeObjectKey = "repo/main/worktree/other"
	if _, _, _, err := CreateOrReuse(ctx, ws, collisionArgs); err == nil {
		t.Fatal("expected collision for same allocation key with different worktree")
	}
}

func mustRootRef(t *testing.T, ctx context.Context, ws world.WorldState, objKey string) *bucket.ObjectRef {
	// Read the persisted allocation root while retaining its object state.
	t.Helper()
	obj, err := world.MustGetObject(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(obj)

	// Require the allocation object to expose its root reference.
	ref, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
