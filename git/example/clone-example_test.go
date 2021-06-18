package git_examples

import (
	"context"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/sirupsen/logrus"
)

func TestCloneExample(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// use in-memory
	if err := RunCloneExample(
		ctx,
		le,
		"../../",
		memory.NewStorage(),
		memfs.New(),
	); err != nil {
		t.Fatal(err.Error())
	}
}
