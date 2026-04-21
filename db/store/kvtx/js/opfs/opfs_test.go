//go:build js

package store_kvtx_opfs

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx/kvtest"
	"github.com/s4wave/spacewave/db/opfs"
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
