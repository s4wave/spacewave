//go:build js

package store_kvtx_opfs

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/kvtx/kvtest"
	"github.com/aperturerobotics/hydra/opfs"
)

func TestOpfsStore(t *testing.T) {
	ctx := context.Background()

	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}

	dir, err := opfs.GetDirectory(root, "test-kvtx", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-kvtx", true) //nolint

	store := NewStore(dir, "test-kvtx|lock")
	if err := kvtx_kvtest.TestAll(ctx, store); err != nil {
		t.Fatal(err)
	}
}
