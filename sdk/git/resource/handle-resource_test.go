package resource_git

import (
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/pkg/errors"
)

// faultRefStorage fails every reference read, as a failed block read would.
type faultRefStorage struct {
	*memory.Storage
}

// errRefRead is the injected reference read failure.
var errRefRead = errors.New("injected reference read failure")

// Reference fails the read.
func (s *faultRefStorage) Reference(plumbing.ReferenceName) (*plumbing.Reference, error) {
	return nil, errRefRead
}

// TestResolveRefNameSeparatesMissingFromFailedReads checks that only a full
// ref the repository lacks reports errRefNameNotFound.
func TestResolveRefNameSeparatesMissingFromFailedReads(t *testing.T) {
	// Commit one empty tree on refs/heads/main.
	store := memory.NewStorage()
	repo, err := git.Init(store)
	if err != nil {
		t.Fatal(err)
	}
	tree := &object.Tree{}
	treeObj := store.NewEncodedObject()
	if err := tree.Encode(treeObj); err != nil {
		t.Fatal(err)
	}
	treeHash, err := store.SetEncodedObject(treeObj)
	if err != nil {
		t.Fatal(err)
	}
	sig := object.Signature{Name: "t", Email: "t@t", When: time.Unix(1, 0)}
	commit := &object.Commit{Author: sig, Committer: sig, Message: "m", TreeHash: treeHash}
	commitObj := store.NewEncodedObject()
	if err := commit.Encode(commitObj); err != nil {
		t.Fatal(err)
	}
	commitHash, err := store.SetEncodedObject(commitObj)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetReference(plumbing.NewHashReference("refs/heads/main", commitHash)); err != nil {
		t.Fatal(err)
	}

	// A held ref resolves and a missing full ref is absent.
	got, err := resolveRefNameToCommitObject(repo, "refs/heads/main")
	if err != nil || got.Hash != commitHash {
		t.Fatalf("resolve main = %v, %v", got, err)
	}
	if _, err := resolveRefNameToCommitObject(repo, "refs/heads/gone"); !errors.Is(err, errRefNameNotFound) {
		t.Fatalf("missing ref error = %v, want errRefNameNotFound", err)
	}

	// A failed reference read stays a read failure.
	faulty, err := git.Open(&faultRefStorage{Storage: store}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolveRefNameToCommitObject(faulty, "refs/heads/main")
	if !errors.Is(err, errRefRead) || errors.Is(err, errRefNameNotFound) {
		t.Fatalf("failed read error = %v, want the read failure", err)
	}
}

func TestBoundedDiffPatchLeavesSmallPatchWhole(t *testing.T) {
	// Prepare a patch that fits within the response byte limit.
	patch := "diff --git a/a.txt b/a.txt\n@@ -1 +1 @@\n-old\n+new\n"

	// Bound the patch using the response byte limit.
	got, truncated, totalBytes := boundedDiffPatch(patch)

	// Verify the small patch and its byte count remain complete.
	if truncated {
		t.Fatal("small patch should not be truncated")
	}
	if got != patch {
		t.Fatalf("patch = %q, want %q", got, patch)
	}
	if totalBytes != uint64(len(patch)) {
		t.Fatalf("total bytes = %d, want %d", totalBytes, len(patch))
	}
}

func TestBoundedDiffPatchTruncatesAtLineBoundary(t *testing.T) {
	// Prepare a patch larger than the response byte limit.
	prefix := "diff --git a/a.txt b/a.txt\n@@ -1 +1 @@\n"
	patch := prefix + strings.Repeat("+0123456789\n", maxDiffPatchBytes/11+2)

	// Bound the patch using the response byte limit.
	got, truncated, totalBytes := boundedDiffPatch(patch)

	// Verify the patch ends at a full line within the byte limit.
	if !truncated {
		t.Fatal("large patch should be truncated")
	}
	if len(got) > maxDiffPatchBytes {
		t.Fatalf("bounded patch length = %d, want <= %d", len(got), maxDiffPatchBytes)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("bounded patch should end on a line boundary: %q", got[len(got)-16:])
	}
	if totalBytes != uint64(len(patch)) {
		t.Fatalf("total bytes = %d, want %d", totalBytes, len(patch))
	}
}
