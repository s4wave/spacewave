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

func TestImportLocalRepoToRefRoundTrip(t *testing.T) {
	ctx, ws := localImportWorld(t)
	path, repo, head := createLocalRepo(t)

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

func TestImportLocalRepoToRefExcludesDirtyFiles(t *testing.T) {
	ctx, ws := localImportWorld(t)
	path, _, head := createLocalRepo(t)
	if err := os.WriteFile(filepath.Join(path, "tracked.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "untracked.txt"), []byte("untracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ref, _, _, err := ImportLocalRepoToRef(ctx, ws, path)
	if err != nil {
		t.Fatalf("ImportLocalRepoToRef: %v", err)
	}
	withImportedStore(t, ctx, ws, ref, func(store *git_block.Store) {
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

func TestImportLocalRepoToRefDetachedHead(t *testing.T) {
	ctx, ws := localImportWorld(t)
	path, repo, head := createLocalRepo(t)
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, head)); err != nil {
		t.Fatalf("detach HEAD: %v", err)
	}

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

func TestImportLocalRepoToRefRejectsUnbornHead(t *testing.T) {
	ctx, ws := localImportWorld(t)
	path := t.TempDir()
	if _, err := git.PlainInit(path, false); err != nil {
		t.Fatal(err)
	}

	ref, _, _, err := ImportLocalRepoToRef(ctx, ws, path)
	if err == nil || ref != nil {
		t.Fatalf("unborn import = (%v, %v), want nil ref and error", ref, err)
	}
}

func TestImportOpenedLocalRepoCancellationDoesNotPublish(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	_, ws := localImportWorld(t)
	_, repo, _ := createLocalRepo(t)
	copied := 0

	ref, _, _, err := importOpenedLocalRepo(ctx, ws, repo, func() {
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

func TestImportOpenedLocalRepoRejectsHeadDrift(t *testing.T) {
	_, ws := localImportWorld(t)
	_, repo, _ := createLocalRepo(t)
	drifting := &headDriftingStorer{Storer: repo.Storer}
	driftingRepo := &git.Repository{Storer: drifting}

	ref, _, _, err := importOpenedLocalRepo(t.Context(), ws, driftingRepo, nil)
	if err == nil || ref != nil {
		t.Fatalf("drifting import = (%v, %v), want nil ref and error", ref, err)
	}
}

func TestImportLocalRepoToRefPreservesAnnotatedTagAndShallowBoundary(t *testing.T) {
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

func TestImportLocalRepoToRefDoesNotMutatePackedReadOnlyGitDir(t *testing.T) {
	ctx, ws := localImportWorld(t)
	path, _, _ := createLocalRepo(t)
	cmd := exec.CommandContext(ctx, "git", "-C", path, "gc", "--prune=now")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pack source repository: %v: %s", err, output)
	}
	gitDir := filepath.Join(path, ".git")
	makeTreeReadOnly(t, gitDir)
	before := snapshotTree(t, gitDir)

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

func TestImportLocalRepoToRefReadsLinkedWorktree(t *testing.T) {
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
	ref, gotHead, branch, err := ImportLocalRepoToRef(ctx, ws, linked)
	if err != nil {
		t.Fatal(err)
	}
	if gotHead != head.String() || branch != "linked" {
		t.Fatalf("imported head/branch = %s/%s, want %s/linked", gotHead, branch, head)
	}
	withImportedStore(t, ctx, ws, ref, func(store *git_block.Store) {
		if _, err := object.GetCommit(store, head); err != nil {
			t.Fatalf("read linked worktree commit: %v", err)
		}
	})
}

func TestImportLocalRepoToRefDoesNotCopyConfiguration(t *testing.T) {
	ctx, ws := localImportWorld(t)
	path, repo, _ := createLocalRepo(t)
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://credential.example.invalid/private.git"},
	}); err != nil {
		t.Fatalf("create source remote: %v", err)
	}

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

func TestImportLocalRepoToRefReadsAlternatesObjectClosure(t *testing.T) {
	ctx, ws := localImportWorld(t)
	path, _, head := createLocalRepo(t)
	sourceObjects := filepath.Join(path, ".git", "objects")
	holderRoot := t.TempDir()
	holderObjects := filepath.Join(holderRoot, "objects")
	if err := os.Rename(sourceObjects, holderObjects); err != nil {
		t.Fatal(err)
	}
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
	cmd := exec.CommandContext(ctx, "git", "-C", path, "cat-file", "-e", "HEAD")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("reference Git cannot read alternate object database: %v: %s", err, output)
	}

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

func TestImportLocalRepoToRefSkipsMissingSubmoduleCommit(t *testing.T) {
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

func localImportWorld(t *testing.T) (context.Context, world.WorldState) {
	t.Helper()
	return localImportWorldWithContext(t, t.Context())
}

func localImportWorldWithContext(t *testing.T, ctx context.Context) (context.Context, world.WorldState) {
	t.Helper()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	return ctx, world.NewEngineWorldState(tb.Engine, true)
}

func createLocalRepo(t *testing.T) (string, *git.Repository, plumbing.Hash) {
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
	signature := &object.Signature{Name: "Test", Email: "test@example.com", When: time.Unix(1, 0).UTC()}
	head, err := worktree.Commit("initial", &git.CommitOptions{Author: signature, Committer: signature})
	if err != nil {
		t.Fatal(err)
	}
	return path, repo, head
}

func withImportedStore(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	ref *bucket.ObjectRef,
	cb func(*git_block.Store),
) {
	t.Helper()
	_, err := world.AccessObject(ctx, ws.AccessWorldState, ref, func(bcs *block.Cursor) error {
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

func forEachReference(t *testing.T, refs storer.ReferenceStorer, cb func(*plumbing.Reference)) {
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

type headDriftingStorer struct {
	storage.Storer
	headReads int
}

func (s *headDriftingStorer) Reference(name plumbing.ReferenceName) (*plumbing.Reference, error) {
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

func makeTreeReadOnly(t *testing.T, root string) {
	t.Helper()
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
	t.Cleanup(func() {
		for _, path := range slices.Backward(paths) {
			_ = os.Chmod(path, 0o700)
		}
	})
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
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
