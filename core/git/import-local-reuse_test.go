package s4wave_git

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/world"
)

// TestImportLocalRepoToRefRetainsPacks checks repeat, advancing, deleted and rewound refs.
func TestImportLocalRepoToRefRetainsPacks(t *testing.T) {
	// Retain the initial import under the same key used by the public entrypoint.
	ctx, ws := localImportWorld(t)
	source, repo, head := createLocalRepo(t)
	key, err := localRepoImportKey(source)
	if err != nil {
		t.Fatal(err)
	}
	first, _, _, err := ImportLocalRepoToRef(ctx, ws, source)
	if err != nil {
		t.Fatal(err)
	}
	initialPacks := importedPacks(t, ctx, ws, first)

	// An unchanged import must not select an object or change the snapshot.
	reads := 0
	second, _, _, err := importOpenedLocalRepo(ctx, ws, repo, key, func() { reads++ })
	if err != nil {
		t.Fatal(err)
	}
	if reads != 0 || !first.GetRootRef().EqualsRef(second.GetRootRef()) {
		t.Fatalf("repeat import: reads=%d, same root=%t", reads, first.GetRootRef().EqualsRef(second.GetRootRef()))
	}

	// Add one commit and repack the source to exercise deltas against retained bases.
	advanced := commitLocalFile(t, source, repo, "advanced\n")
	if err := repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/retained", head)); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, "git", "-C", source, "gc", "--prune=now").CombinedOutput(); err != nil {
		t.Fatalf("repack source: %v: %s", err, output)
	}

	// Reopen the fixture after Git replaced its object database during repacking.
	if err := repo.Storer.(io.Closer).Close(); err != nil {
		t.Fatal(err)
	}
	repo, err = git.PlainOpen(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repo.Storer.(io.Closer).Close(); err != nil {
			t.Error(err)
		}
	})

	// Import the advanced source with its fresh object database handles.
	third, gotHead, _, err := ImportLocalRepoToRef(ctx, ws, source)
	if err != nil || gotHead != advanced.String() {
		t.Fatalf("advanced import: head=%s, err=%v", gotHead, err)
	}

	// The second pack holds only the new commit, tree and blob; the first is shared.
	advancedPacks := importedPacks(t, ctx, ws, third)
	if len(initialPacks) != 1 || len(advancedPacks) != 2 {
		t.Fatalf("pack counts: initial=%d, advanced=%d", len(initialPacks), len(advancedPacks))
	}
	for hash, pack := range initialPacks {
		if !pack.EqualVT(advancedPacks[hash]) {
			t.Fatalf("retained pack %s changed", hash)
		}
	}
	for hash, pack := range advancedPacks {
		if initialPacks[hash] == nil && pack.GetObjectCount() != 3 {
			t.Fatalf("incremental pack objects=%d, want 3", pack.GetObjectCount())
		}
	}

	// Deleting a source ref changes only references, never the existing packs.
	if err := repo.Storer.RemoveReference("refs/heads/retained"); err != nil {
		t.Fatal(err)
	}
	reads = 0
	fourth, _, _, err := importOpenedLocalRepo(ctx, ws, repo, key, func() { reads++ })
	if err != nil || reads != 0 {
		t.Fatalf("reference deletion: reads=%d, err=%v", reads, err)
	}
	withImportedStore(t, ctx, ws, fourth, func(store *git_block.Store) {
		if _, err := store.Reference("refs/heads/retained"); !errors.Is(err, plumbing.ErrReferenceNotFound) {
			t.Fatalf("deleted reference remains: %v", err)
		}
	})

	// Rewinding to retained history must encode no objects and leave old snapshots intact.
	if err := repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/master", head)); err != nil {
		t.Fatal(err)
	}
	reads = 0
	fifth, _, _, err := importOpenedLocalRepo(ctx, ws, repo, key, func() { reads++ })
	if err != nil || reads != 0 {
		t.Fatalf("rewound import: reads=%d, err=%v", reads, err)
	}
	for _, snapshot := range []struct {
		// ref selects the immutable import to inspect.
		ref *bucket.ObjectRef
		// head is the captured revision that must remain stable.
		head plumbing.Hash
	}{{first, head}, {third, advanced}, {fifth, head}} {
		withImportedStore(t, ctx, ws, snapshot.ref, func(store *git_block.Store) {
			got, err := storer.ResolveReference(store, plumbing.HEAD)
			if err != nil || got.Hash() != snapshot.head {
				t.Fatalf("immutable snapshot HEAD=%v, err=%v, want %s", got, err, snapshot.head)
			}
		})
	}
}

// TestImportLocalRepoToRefDeepensRetainedHistory checks boundary changes on indexed commits.
func TestImportLocalRepoToRefDeepensRetainedHistory(t *testing.T) {
	// Import only the tip of a two-commit source history.
	ctx, ws := localImportWorld(t)
	source, repo, parent := createLocalRepo(t)
	head := commitLocalFile(t, source, repo, "tip\n")
	if err := repo.Storer.SetShallow([]plumbing.Hash{head}); err != nil {
		t.Fatal(err)
	}
	first, _, _, err := ImportLocalRepoToRef(ctx, ws, source)
	if err != nil {
		t.Fatal(err)
	}
	withImportedStore(t, ctx, ws, first, func(store *git_block.Store) {
		if err := store.HasEncodedObject(parent); !errors.Is(err, plumbing.ErrObjectNotFound) {
			t.Fatalf("shallow parent presence: %v", err)
		}
	})

	// Change to a complete orphan graph, retaining the old shallow tip as an object only.
	orphan := &object.Commit{Author: object.Signature{Name: "Test", Email: "test@example.com"}, Message: "orphan"}
	original, err := repo.CommitObject(head)
	if err != nil {
		t.Fatal(err)
	}
	orphan.TreeHash = original.TreeHash
	encoded := repo.Storer.NewEncodedObject()
	if err := orphan.Encode(encoded); err != nil {
		t.Fatal(err)
	}
	orphanHash, err := repo.Storer.SetEncodedObject(encoded)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the orphan with no shallow boundary while the original parent stays absent.
	if err := repo.Storer.SetShallow(nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/master", orphanHash)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ImportLocalRepoToRef(ctx, ws, source); err != nil {
		t.Fatal(err)
	}

	// A retained tip is not proof of its closure under the current boundary.
	if err := repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/master", head)); err != nil {
		t.Fatal(err)
	}
	deepened, _, _, err := ImportLocalRepoToRef(ctx, ws, source)
	if err != nil {
		t.Fatal(err)
	}
	withImportedStore(t, ctx, ws, deepened, func(store *git_block.Store) {
		if _, err := object.GetCommit(store, parent); err != nil {
			t.Fatalf("deepened parent absent: %v", err)
		}
	})
}

// TestImportLocalRepoToRefCanceledExtensionKeepsSnapshot checks atomic cache publication.
func TestImportLocalRepoToRefCanceledExtensionKeepsSnapshot(t *testing.T) {
	// Retain one committed snapshot before attempting a canceled extension.
	ctx, ws := localImportWorld(t)
	source, repo, _ := createLocalRepo(t)
	key, err := localRepoImportKey(source)
	if err != nil {
		t.Fatal(err)
	}
	first, _, _, err := ImportLocalRepoToRef(ctx, ws, source)
	if err != nil {
		t.Fatal(err)
	}
	commitLocalFile(t, source, repo, "canceled\n")

	// Cancel selection of the new graph before its root can be published.
	canceled, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	ref, _, _, err := importOpenedLocalRepo(canceled, ws, repo, key, cancel)
	if !errors.Is(err, context.Canceled) || ref != nil {
		t.Fatalf("canceled extension: ref=%v, err=%v", ref, err)
	}

	// The retained World object still names the last verified snapshot.
	state, exists, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(state)
	if err != nil || !exists {
		t.Fatalf("retained import: exists=%t, err=%v", exists, err)
	}
	current, _, err := state.GetRootRef(ctx)
	if err != nil || !current.GetRootRef().EqualsRef(first.GetRootRef()) {
		t.Fatalf("canceled extension changed retained root: %v", err)
	}
}

// TestImportLocalRepoToRefConcurrentFirstImport preserves both completed snapshots.
func TestImportLocalRepoToRefConcurrentFirstImport(t *testing.T) {
	// Start an import before the source has any retained World object.
	ctx, ws := localImportWorld(t)
	source, repo, head := createLocalRepo(t)
	key, err := localRepoImportKey(source)
	if err != nil {
		t.Fatal(err)
	}

	// Finish a competing import after the first selected object, before publication.
	var winner *bucket.ObjectRef
	ref, gotHead, _, err := importOpenedLocalRepo(ctx, ws, repo, key, func() {
		if winner != nil {
			return
		}
		var err error
		winner, _, _, err = ImportLocalRepoToRef(ctx, ws, source)
		if err != nil {
			t.Fatal(err)
		}
	})
	if err != nil || gotHead != head.String() {
		t.Fatalf("concurrent import: head=%s, err=%v", gotHead, err)
	}

	// The losing creator still returns a complete immutable repository snapshot.
	withImportedStore(t, ctx, ws, ref, func(store *git_block.Store) {
		if _, err := object.GetCommit(store, head); err != nil {
			t.Fatal(err)
		}
	})
	repeated, _, _, err := ImportLocalRepoToRef(ctx, ws, source)
	if err != nil {
		t.Fatal(err)
	}
	if !winner.GetRootRef().EqualsRef(repeated.GetRootRef()) {
		t.Fatal("competing import replaced the retained snapshot")
	}
}

// commitLocalFile advances the fixture with one new commit, tree and blob.
func commitLocalFile(t *testing.T, source string, repo *git.Repository, contents string) plumbing.Hash {
	// Stage the changed tracked fixture file.
	t.Helper()
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("tracked.txt"); err != nil {
		t.Fatal(err)
	}

	// Return the committed revision with a fixed identity and timestamp.
	signature := &object.Signature{Name: "Test", Email: "test@example.com", When: time.Unix(2, 0).UTC()}
	head, err := worktree.Commit(contents, &git.CommitOptions{Author: signature, Committer: signature})
	if err != nil {
		t.Fatal(err)
	}
	return head
}

// importedPacks reads pack metadata without copying pack data.
func importedPacks(t *testing.T, ctx context.Context, ws world.WorldState, ref *bucket.ObjectRef) map[plumbing.Hash]*git_block.Packfile {
	// Visit every retained pack in the immutable snapshot.
	t.Helper()
	packs := make(map[plumbing.Hash]*git_block.Packfile)
	withImportedStore(t, ctx, ws, ref, func(store *git_block.Store) {
		// Open the pack metadata tree and release its iterator after decoding.
		objects, cs, err := store.GetRoot().FollowEncodedObjectStore(ctx, store.GetCursor())
		if err != nil {
			t.Fatal(err)
		}
		tree, err := objects.BuildPackfileTree(ctx, cs)
		if err != nil {
			t.Fatal(err)
		}
		iter := tree.BlockIterate(ctx, nil, false, false)
		defer iter.Close()

		// Retain only metadata for pack-sharing and object-count assertions.
		for iter.Next() {
			pack, err := block.UnmarshalBlock[*git_block.Packfile](ctx, iter.ValueCursor(), git_block.NewPackfileBlock)
			if err != nil {
				t.Fatal(err)
			}
			hash, err := git_block.FromHash(pack.GetPackHash())
			if err != nil {
				t.Fatal(err)
			}
			packs[hash] = pack.CloneVT()
		}
		if err := iter.Err(); err != nil {
			t.Fatal(err)
		}
	})
	return packs
}
