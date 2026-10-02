package s4wave_git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/go-git/go-git/v6/storage"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// TestImportLocalRepoToRefRoundTrip preserves HEAD and every committed reference.
func TestImportLocalRepoToRefRoundTrip(t *testing.T) {
	// Build the World testbed and a local repository to import.
	ctx, ws := localImportWorld(t)
	path, repo, head := createLocalRepo(t)

	// Import the local repository and verify the head and branch.
	ref, gotHead, branch, err := ImportLocalRepoToRef(ctx, ws, path)
	if err != nil {
		t.Fatalf("ImportLocalRepoToRef: %v", err)
	}
	if gotHead != head.String() {
		t.Fatalf("head = %q, want %q", gotHead, head)
	}
	if branch != "master" {
		t.Fatalf("branch = %q, want master", branch)
	}

	// Open the imported store and verify its HEAD and references.
	withImportedStore(t, ctx, ws, ref, func(store *git_block.Store) {
		got, err := storer.ResolveReference(store, plumbing.HEAD)
		if err != nil {
			t.Fatalf("resolve imported HEAD: %v", err)
		}
		if got.Hash() != head {
			t.Fatalf("imported HEAD = %s, want %s", got.Hash(), head)
		}
		forEachReference(t, repo.Storer, func(source *plumbing.Reference) {
			imported, err := store.Reference(source.Name())
			if err != nil {
				t.Fatalf("read imported reference %s: %v", source.Name(), err)
			}
			if !sameReference(source, imported) {
				t.Fatalf("imported reference %s = %s, want %s", source.Name(), imported, source)
			}
		})
	})
}

// TestImportLocalRepoToRefExcludesDirtyFiles imports only committed file contents.
func TestImportLocalRepoToRefExcludesDirtyFiles(t *testing.T) {
	// Build the World testbed and a repository with dirty and untracked files.
	ctx, ws := localImportWorld(t)
	path, _, head := createLocalRepo(t)
	if err := os.WriteFile(filepath.Join(path, "tracked.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "untracked.txt"), []byte("untracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Import the repository and verify only committed content was copied.
	ref, _, _, err := ImportLocalRepoToRef(ctx, ws, path)
	if err != nil {
		t.Fatalf("ImportLocalRepoToRef: %v", err)
	}
	withImportedStore(t, ctx, ws, ref, func(store *git_block.Store) {
		// Read the imported commit and verify its tracked contents.
		commit, err := object.GetCommit(store, head)
		if err != nil {
			t.Fatalf("read imported commit: %v", err)
		}
		file, err := commit.File("tracked.txt")
		if err != nil {
			t.Fatalf("read imported tracked file: %v", err)
		}
		contents, err := file.Contents()
		if err != nil {
			t.Fatalf("read imported tracked contents: %v", err)
		}
		if contents != "committed\n" {
			t.Fatalf("imported tracked contents = %q", contents)
		}
	})
}

// TestImportLocalRepoToRefDetachedHead preserves a detached HEAD without inventing a branch.
func TestImportLocalRepoToRefDetachedHead(t *testing.T) {
	// Build the testbed and detach the repository's HEAD.
	ctx, ws := localImportWorld(t)
	path, repo, head := createLocalRepo(t)
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, head)); err != nil {
		t.Fatalf("detach HEAD: %v", err)
	}

	// Import the detached repository and verify the empty branch and head.
	ref, gotHead, branch, err := ImportLocalRepoToRef(ctx, ws, path)
	if err != nil {
		t.Fatalf("ImportLocalRepoToRef: %v", err)
	}
	if gotHead != head.String() || branch != "" {
		t.Fatalf("detached result = (%q, %q), want (%q, empty)", gotHead, branch, head)
	}
	withImportedStore(t, ctx, ws, ref, func(store *git_block.Store) {
		raw, err := store.Reference(plumbing.HEAD)
		if err != nil {
			t.Fatal(err)
		}
		if raw.Type() != plumbing.HashReference || raw.Hash() != head {
			t.Fatalf("imported detached HEAD = %s", raw)
		}
	})
}

// TestImportLocalRepoToRefRejectsUnbornHead rejects repositories with no committed HEAD.
func TestImportLocalRepoToRefRejectsUnbornHead(t *testing.T) {
	// Build the testbed and initialize a repository with no commits.
	ctx, ws := localImportWorld(t)
	path := t.TempDir()
	if _, err := git.PlainInit(path, false); err != nil {
		t.Fatal(err)
	}

	// Import the unborn repository and expect a nil ref with an error.
	ref, _, _, err := ImportLocalRepoToRef(ctx, ws, path)
	if err == nil || ref != nil {
		t.Fatalf("unborn import = (%v, %v), want nil ref and error", ref, err)
	}
}

// TestImportOpenedLocalRepoCancellationDoesNotPublish rejects partially encoded imports.
func TestImportOpenedLocalRepoCancellationDoesNotPublish(t *testing.T) {
	// Build the testbed, repository, and a cancellable context.
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	_, ws := localImportWorld(t)
	_, repo, _ := createLocalRepo(t)
	copied := 0

	// Cancel during the first copied object and expect no published ref.
	ref, _, _, err := importOpenedLocalRepo(ctx, ws, repo, "git/local-import/canceled", func() {
		copied++
		cancel()
	})
	if err == nil || ref != nil {
		t.Fatalf("canceled import = (%v, %v), want nil ref and error", ref, err)
	}
	if copied != 1 {
		t.Fatalf("copied objects before cancellation = %d, want 1", copied)
	}
}

// TestImportOpenedLocalRepoRejectsHeadDrift fences publication against a source checkout change.
func TestImportOpenedLocalRepoRejectsHeadDrift(t *testing.T) {
	// Build the testbed and a storer whose HEAD drifts after repeated reads.
	_, ws := localImportWorld(t)
	_, repo, _ := createLocalRepo(t)
	drifting := &headDriftingStorer{Storer: repo.Storer}
	driftingRepo := &git.Repository{Storer: drifting}

	// Import the drifting repository and expect a nil ref with an error.
	ref, _, _, err := importOpenedLocalRepo(t.Context(), ws, driftingRepo, "git/local-import/drifted", nil)
	if err == nil || ref != nil {
		t.Fatalf("drifting import = (%v, %v), want nil ref and error", ref, err)
	}
}

// TestImportLocalRepoToRefPreservesAnnotatedTagAndShallowBoundary preserves tags and shallow boundaries.
func TestImportLocalRepoToRefPreservesAnnotatedTagAndShallowBoundary(t *testing.T) {
	// Build the testbed and add an annotated tag plus shallow boundary.
	ctx, ws := localImportWorld(t)
	path, repo, head := createLocalRepo(t)
	signature := &object.Signature{Name: "Test", Email: "test@example.com", When: time.Unix(2, 0).UTC()}
	tag, err := repo.CreateTag("v1", head, &git.CreateTagOptions{Tagger: signature, Message: "release"})
	if err != nil {
		t.Fatalf("create annotated tag: %v", err)
	}
	if err := repo.Storer.SetShallow([]plumbing.Hash{head}); err != nil {
		t.Fatalf("set shallow boundary: %v", err)
	}

	// Import the repository and verify the tag and shallow boundary survived.
	ref, _, _, err := ImportLocalRepoToRef(ctx, ws, path)
	if err != nil {
		t.Fatalf("ImportLocalRepoToRef: %v", err)
	}
	withImportedStore(t, ctx, ws, ref, func(store *git_block.Store) {
		if _, err := store.EncodedObject(plumbing.TagObject, tag.Hash()); err != nil {
			t.Fatalf("read imported tag object: %v", err)
		}
		shallow, err := store.Shallow()
		if err != nil {
			t.Fatalf("read imported shallow boundary: %v", err)
		}
		if len(shallow) != 1 || shallow[0] != head {
			t.Fatalf("imported shallow boundary = %v, want [%s]", shallow, head)
		}
	})
}

// TestImportLocalRepoToRefDoesNotMutatePackedReadOnlyGitDir keeps the source object database read-only.
func TestImportLocalRepoToRefDoesNotMutatePackedReadOnlyGitDir(t *testing.T) {
	// Pack the source repository and snapshot its read-only Git directory.
	ctx, ws := localImportWorld(t)
	path, _, _ := createLocalRepo(t)
	cmd := exec.CommandContext(ctx, "git", "-C", path, "gc", "--prune=now")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pack source repository: %v: %s", err, output)
	}
	gitDir := filepath.Join(path, ".git")
	makeTreeReadOnly(t, gitDir)
	before := snapshotTree(t, gitDir)

	// Import the packed repository and verify the Git directory is unchanged.
	if _, _, _, err := ImportLocalRepoToRef(ctx, ws, path); err != nil {
		t.Fatalf("ImportLocalRepoToRef: %v", err)
	}
	after := snapshotTree(t, gitDir)
	if len(after) != len(before) {
		t.Fatalf("source Git tree entry count changed: %d -> %d", len(before), len(after))
	}
	for path, digest := range before {
		if after[path] != digest {
			t.Fatalf("source Git path %q changed", path)
		}
	}
}

// TestImportLocalRepoToRefReadsLinkedWorktree preserves the linked HEAD and shared objects.
func TestImportLocalRepoToRefReadsLinkedWorktree(t *testing.T) {
	// Create a packed source and a linked checkout with its own branch.
	ctx, ws := localImportWorld(t)
	path, _, head := createLocalRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	for _, args := range [][]string{
		{"-C", path, "gc", "--prune=now"},
		{"-C", path, "worktree", "add", "-b", "linked", linked},
	} {
		if output, err := exec.CommandContext(ctx, "git", args...).CombinedOutput(); err != nil {
			t.Fatalf("prepare linked worktree: %v: %s", err, output)
		}
	}

	// Import the linked checkout through its gitfile and common object database.
	ref, gotHead, branch, err := ImportLocalRepoToRef(ctx, ws, linked)
	if err != nil {
		t.Fatal(err)
	}
	if gotHead != head.String() || branch != "linked" {
		t.Fatalf("imported head/branch = %s/%s, want %s/linked", gotHead, branch, head)
	}

	// Resolve the captured commit through the stored pack.
	withImportedStore(t, ctx, ws, ref, func(store *git_block.Store) {
		if _, err := object.GetCommit(store, head); err != nil {
			t.Fatalf("read linked worktree commit: %v", err)
		}
	})
}

// TestImportLocalRepoToRefDoesNotCopyConfiguration excludes local remotes and their configuration.
func TestImportLocalRepoToRefDoesNotCopyConfiguration(t *testing.T) {
	// Build the testbed and add a remote with credentials to the source repo.
	ctx, ws := localImportWorld(t)
	path, repo, _ := createLocalRepo(t)
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://credential.example.invalid/private.git"},
	}); err != nil {
		t.Fatalf("create source remote: %v", err)
	}

	// Import the repository and verify no remotes were copied.
	ref, _, _, err := ImportLocalRepoToRef(ctx, ws, path)
	if err != nil {
		t.Fatalf("ImportLocalRepoToRef: %v", err)
	}
	withImportedStore(t, ctx, ws, ref, func(store *git_block.Store) {
		conf, err := store.Config()
		if err != nil {
			t.Fatalf("read imported config: %v", err)
		}
		if len(conf.Remotes) != 0 {
			t.Fatalf("imported config contains source remotes: %#v", conf.Remotes)
		}
	})
}

// TestImportLocalRepoToRefReadsAlternatesObjectClosure includes objects supplied by alternates.
func TestImportLocalRepoToRefReadsAlternatesObjectClosure(t *testing.T) {
	// Build the testbed and move the source objects into an alternates holder.
	ctx, ws := localImportWorld(t)
	path, _, head := createLocalRepo(t)
	sourceObjects := filepath.Join(path, ".git", "objects")
	holderRoot := t.TempDir()
	holderObjects := filepath.Join(holderRoot, "objects")
	if err := os.Rename(sourceObjects, holderObjects); err != nil {
		t.Fatal(err)
	}

	// Point the source objects directory at the holder via an alternates file.
	if err := os.MkdirAll(filepath.Join(sourceObjects, "info"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(sourceObjects, "info", "alternates"),
		[]byte(holderObjects+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	// Verify reference Git can read the alternate object database.
	cmd := exec.CommandContext(ctx, "git", "-C", path, "cat-file", "-e", "HEAD")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("reference Git cannot read alternate object database: %v: %s", err, output)
	}

	// Import through the alternates file and verify the head resolves.
	ref, gotHead, _, err := ImportLocalRepoToRef(ctx, ws, path)
	if err != nil {
		t.Fatalf("ImportLocalRepoToRef: %v", err)
	}
	if gotHead != head.String() {
		t.Fatalf("head = %q, want %s", gotHead, head)
	}
	withImportedStore(t, ctx, ws, ref, func(store *git_block.Store) {
		if _, err := object.GetCommit(store, head); err != nil {
			t.Fatalf("read alternates-backed commit: %v", err)
		}
	})
}

// TestImportLocalRepoToRefSkipsMissingSubmoduleCommit treats gitlinks as external repository roots.
func TestImportLocalRepoToRefSkipsMissingSubmoduleCommit(t *testing.T) {
	// Build the testbed and encode a tree with a missing submodule gitlink.
	ctx, ws := localImportWorld(t)
	path, repo, _ := createLocalRepo(t)
	signature := object.Signature{Name: "Test", Email: "test@example.com", When: time.Unix(3, 0).UTC()}
	tree := &object.Tree{Entries: []object.TreeEntry{{
		Name: "submodule",
		Mode: filemode.Submodule,
		Hash: plumbing.NewHash("1111111111111111111111111111111111111111"),
	}}}
	treeObject := repo.Storer.NewEncodedObject()
	if err := tree.Encode(treeObject); err != nil {
		t.Fatal(err)
	}
	treeHash, err := repo.Storer.SetEncodedObject(treeObject)
	if err != nil {
		t.Fatal(err)
	}

	// Encode the gitlink commit and point master at it.
	commit := &object.Commit{
		Author: signature, Committer: signature, Message: "gitlink", TreeHash: treeHash,
	}
	commitObject := repo.Storer.NewEncodedObject()
	if err := commit.Encode(commitObject); err != nil {
		t.Fatal(err)
	}
	commitHash, err := repo.Storer.SetEncodedObject(commitObject)
	if err != nil {
		t.Fatal(err)
	}
	branch := plumbing.NewHashReference(plumbing.ReferenceName("refs/heads/master"), commitHash)
	if err := repo.Storer.SetReference(branch); err != nil {
		t.Fatal(err)
	}

	// Import the repository and verify the gitlink commit was stored.
	ref, gotHead, _, err := ImportLocalRepoToRef(ctx, ws, path)
	if err != nil {
		t.Fatalf("ImportLocalRepoToRef: %v", err)
	}
	if gotHead != commitHash.String() {
		t.Fatalf("head = %q, want %s", gotHead, commitHash)
	}
	withImportedStore(t, ctx, ws, ref, func(store *git_block.Store) {
		if _, err := object.GetCommit(store, commitHash); err != nil {
			t.Fatalf("read imported gitlink commit: %v", err)
		}
		if _, err := store.EncodedObject(plumbing.AnyObject, plumbing.NewHash("1111111111111111111111111111111111111111")); err == nil {
			t.Fatal("missing submodule commit was unexpectedly imported")
		}
	})
}

// localImportWorld starts an import testbed bound to the test lifetime.
func localImportWorld(t *testing.T) (context.Context, world.WorldState) {
	t.Helper()
	return localImportWorldWithContext(t, t.Context())
}

// localImportWorldWithContext starts a writable World for retained source imports.
func localImportWorldWithContext(t *testing.T, ctx context.Context) (context.Context, world.WorldState) {
	// Start a default World testbed for the import tests.
	t.Helper()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	return ctx, world.NewEngineWorldState(tb.Engine, true)
}

// createLocalRepo creates a committed local repository fixture.
func createLocalRepo(t *testing.T) (string, *git.Repository, plumbing.Hash) {
	// Initialize a repository and write a tracked file.
	t.Helper()
	path := t.TempDir()
	repo, err := git.PlainInit(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "tracked.txt"), []byte("committed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("tracked.txt"); err != nil {
		t.Fatal(err)
	}

	// Commit the tracked file and return the head hash.
	signature := &object.Signature{Name: "Test", Email: "test@example.com", When: time.Unix(1, 0).UTC()}
	head, err := worktree.Commit("initial", &git.CommitOptions{Author: signature, Committer: signature})
	if err != nil {
		t.Fatal(err)
	}
	return path, repo, head
}

// withImportedStore opens an immutable import snapshot for assertions.
func withImportedStore(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	ref *bucket.ObjectRef,
	cb func(*git_block.Store),
) {
	t.Helper()
	// Open the imported repository through the object ref and run the callback.
	_, err := world.AccessObject(ctx, ws.AccessWorldState, ref, func(bcs *block.Cursor) error {
		// Unmarshal the repository and open a git_block.Store over its cursor.
		if _, err := git_block.UnmarshalRepo(ctx, bcs); err != nil {
			return err
		}
		store, err := git_block.NewStore(ctx, nil, bcs, &memory.IndexStorage{}, nil)
		if err != nil {
			return err
		}
		defer store.Close()
		cb(store)
		return nil
	})
	if err != nil {
		t.Fatalf("open imported repository: %v", err)
	}
}

// forEachReference visits all references and releases its iterator.
func forEachReference(t *testing.T, refs storer.ReferenceStorer, cb func(*plumbing.Reference)) {
	// Iterate every reference and invoke the callback for each.
	t.Helper()
	iter, err := refs.IterReferences()
	if err != nil {
		t.Fatal(err)
	}
	defer iter.Close()
	if err := iter.ForEach(func(ref *plumbing.Reference) error {
		cb(ref)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// headDriftingStorer changes HEAD during import to test the publication fence.
type headDriftingStorer struct {
	// Storer supplies the stable source graph.
	storage.Storer
	// headReads counts observations before changing HEAD.
	headReads int
}

// Reference changes HEAD after the snapshot has been captured.
func (s *headDriftingStorer) Reference(name plumbing.ReferenceName) (*plumbing.Reference, error) {
	// Serve HEAD reads that drift to a symbolic ref after repeated lookups.
	ref, err := s.Storer.Reference(name)
	if err != nil || name != plumbing.HEAD {
		return ref, err
	}
	s.headReads++
	if s.headReads >= 3 {
		return plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/drifted")), nil
	}
	return ref, nil
}

// makeTreeReadOnly revokes writes and restores permissions during cleanup.
func makeTreeReadOnly(t *testing.T, root string) {
	t.Helper()

	// Walk the tree and revoke write permission from every entry.
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		if entry.IsDir() {
			return os.Chmod(path, 0o500)
		}
		return os.Chmod(path, 0o400)
	}); err != nil {
		t.Fatal(err)
	}

	// Restore write permission to every entry after the test.
	t.Cleanup(func() {
		for _, path := range slices.Backward(paths) {
			_ = os.Chmod(path, 0o700)
		}
	})
}

// snapshotTree records filesystem contents for read-only import assertions.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	// Walk the tree and record a digest for every entry.
	out := make(map[string]string)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		// Resolve each entry relative to the root and digest its contents.
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			out[rel] = "directory"
			return nil
		}

		// Read each file's contents and record its SHA-256 digest.
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(contents)
		out[rel] = hex.EncodeToString(digest[:])
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}
