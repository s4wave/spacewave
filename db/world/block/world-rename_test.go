package world_block_test

import (
	"context"
	"slices"
	"testing"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TestWorldState_RenameObject tests object key rename behavior.
func TestWorldState_RenameObject(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	oldKey := "rename-old"
	newKey := "rename-new"
	otherKey := "rename-other"
	rootRef := &bucket.ObjectRef{BucketId: "test-bucket"}
	if _, err := ws.CreateObject(ctx, oldKey, rootRef); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := ws.CreateObject(ctx, otherKey, rootRef); err != nil {
		t.Fatal(err.Error())
	}

	oldValue := world.KeyToGraphValue(oldKey).String()
	newValue := world.KeyToGraphValue(newKey).String()
	otherValue := world.KeyToGraphValue(otherKey).String()
	for _, q := range []world.GraphQuad{
		world.NewGraphQuad(oldValue, "<predicate1>", otherValue, ""),
		world.NewGraphQuad(otherValue, "<predicate2>", oldValue, ""),
		world.NewGraphQuad(oldValue, "<predicate3>", oldValue, ""),
	} {
		if err := ws.SetGraphQuad(ctx, q); err != nil {
			t.Fatal(err.Error())
		}
	}

	oldObj, err := world.MustGetObject(ctx, ws, oldKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	oldRootRef, oldRev, err := oldObj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	renamed, err := ws.RenameObject(ctx, oldKey, newKey, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	newRootRef, newRev, err := renamed.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !oldRootRef.EqualVT(newRootRef) {
		t.Fatalf("expected root ref to be preserved")
	}
	if oldRev != newRev {
		t.Fatalf("expected rev %d to be preserved, got %d", oldRev, newRev)
	}

	_, err = ws.RenameObject(ctx, newKey, otherKey, false)
	if !errors.Is(err, world.ErrObjectExists) {
		t.Fatalf("expected ErrObjectExists, got %v", err)
	}
	_, err = ws.RenameObject(ctx, "rename-missing", "rename-unused", false)
	if !errors.Is(err, world.ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound, got %v", err)
	}
	_, err = ws.RenameObject(ctx, "rename-missing", otherKey, true)
	if !errors.Is(err, world.ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound, got %v", err)
	}

	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	if _, found, err := ws.GetObject(ctx, oldKey); err != nil {
		t.Fatal(err.Error())
	} else if found {
		t.Fatalf("expected old key %q to be absent", oldKey)
	}
	if _, found, err := ws.GetObject(ctx, newKey); err != nil {
		t.Fatal(err.Error())
	} else if !found {
		t.Fatalf("expected new key %q to exist", newKey)
	}

	oldSubj, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad(oldValue, "", "", ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	oldObjQuads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad("", "", oldValue, ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(oldSubj) != 0 || len(oldObjQuads) != 0 {
		t.Fatalf("expected no graph quads referencing old key")
	}
	newSubj, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad(newValue, "", "", ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	newObjQuads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad("", "", newValue, ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(newSubj) != 2 || len(newObjQuads) != 2 {
		t.Fatalf("expected rewritten graph quads for new key, got subj=%d obj=%d", len(newSubj), len(newObjQuads))
	}

	worldRoot, err := ws.GetRoot(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	lastChange := worldRoot.GetLastChange()
	if ct := lastChange.GetChangeType(); ct != world_block.WorldChangeType_WorldChange_OBJECT_RENAME {
		t.Fatalf("expected last change type OBJECT_RENAME, got %s", ct.String())
	}
	changes := lastChange.GetChangeBatch().GetChanges()
	if len(changes) != 1 {
		t.Fatalf("expected one object rename change, got %d", len(changes))
	}
	if changes[0].GetKey() != oldKey || changes[0].GetNewKey() != newKey {
		t.Fatalf("expected rename change %q -> %q, got %q -> %q", oldKey, newKey, changes[0].GetKey(), changes[0].GetNewKey())
	}

	rg := ws.GetRefGraph()
	if rg == nil {
		t.Fatal("expected ref graph")
	}
	worldRefs, err := rg.GetOutgoingRefs(ctx, "world")
	if err != nil {
		t.Fatal(err.Error())
	}
	if slices.Contains(worldRefs, block_gc.ObjectIRI(oldKey)) {
		t.Fatalf("expected world refs not to contain old object iri")
	}
	if !slices.Contains(worldRefs, block_gc.ObjectIRI(newKey)) {
		t.Fatalf("expected world refs to contain new object iri")
	}
}

// TestWorldState_RenameGitRepoWithWizardChildren tests renaming the parent
// object shape created by the Git repository clone wizard.
func TestWorldState_RenameGitRepoWithWizardChildren(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	repoKey := "repo-1"
	workdirKey := repoKey + "/workdir"
	worktreeKey := repoKey + "/worktree"
	for _, key := range []string{repoKey, workdirKey, worktreeKey} {
		if _, err := ws.CreateObject(ctx, key, &bucket.ObjectRef{BucketId: key}); err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := world_types.SetObjectType(ctx, ws, repoKey, git_world.GitRepoTypeID); err != nil {
		t.Fatal(err.Error())
	}
	if err := world_types.SetObjectType(ctx, ws, worktreeKey, git_world.GitWorktreeTypeID); err != nil {
		t.Fatal(err.Error())
	}
	if err := world_parent.SetObjectParent(ctx, ws, workdirKey, repoKey, false); err != nil {
		t.Fatal(err.Error())
	}
	if err := world_parent.SetObjectParent(ctx, ws, worktreeKey, repoKey, false); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(worktreeKey, git_world.GitRepoPred, repoKey, "")); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(repoKey, git_world.GitRepoWorktreePred, worktreeKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	if _, err := ws.RenameObject(ctx, repoKey, "myrepo", true); err != nil {
		t.Fatal(err.Error())
	}

	for _, key := range []string{repoKey, workdirKey, worktreeKey} {
		if _, found, err := ws.GetObject(ctx, key); err != nil {
			t.Fatal(err.Error())
		} else if found {
			t.Fatalf("expected old key %q to be absent", key)
		}
	}
	for _, key := range []string{"myrepo", "myrepo/workdir", "myrepo/worktree"} {
		if _, found, err := ws.GetObject(ctx, key); err != nil {
			t.Fatal(err.Error())
		} else if !found {
			t.Fatalf("expected new key %q to exist", key)
		}
	}
}
