package kvtx_kvtest

import (
	"context"
	"testing"

	sinmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
)

func TestKVTest(t *testing.T) {
	ctx := context.Background()
	store := sinmem.NewStore()
	if err := TestAll(ctx, store); err != nil {
		t.Fatal(err.Error())
	}
}
