//go:build e2e

package git_block

import (
	"context"
	"testing"

	git_examples "github.com/s4wave/spacewave/db/git/example"
	"github.com/s4wave/spacewave/db/testbed"
	billy "github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/sirupsen/logrus"
)

func buildStore(t *testing.T, ctx context.Context, tb *testbed.Testbed) (billy.Filesystem, *Store) {
	oc, _ := tb.BuildEmptyCursor(ctx)
	inMem := memory.NewStorage()

	btx, bcs := oc.BuildTransaction(nil)
	root := NewRepo()
	bcs.SetBlock(root, true)
	store, err := NewStore(ctx, btx, bcs, inMem, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	worktree := memfs.New()
	return worktree, store
}

// TestStorage_Clone runs the git storage end to end test.
func TestStorage_Clone(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	testbed.Verbose = false
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	worktree, store := buildStore(t, subCtx, tb)
	err = git_examples.RunCloneExample(
		ctx,
		le,
		"https://github.com/pkg/errors",
		store, worktree,
	)
	// err = git_examples.RunCloneExample(ctx, le, "https://github.com/pkg/errors", inMem, worktree)
	if err == nil {
		err = store.Commit()
	}
	if err != nil {
		t.Fatal(err.Error())
	}
	store.Close()
}
