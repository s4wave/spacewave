//go:build e2e

package git_examples

import (
	"context"
	"testing"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/sirupsen/logrus"
)

func TestCloneExample(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	repoPath := "https://github.com/pkg/errors"

	// Use in-memory storage and an in-memory worktree.
	if err := RunCloneExample(
		ctx,
		le,
		repoPath,
		memory.NewStorage(),
		memfs.New(),
	); err != nil {
		t.Fatal(err.Error())
	}
}
