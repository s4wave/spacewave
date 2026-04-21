package kvtx_prefixer

import (
	"context"
	"testing"

	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	sinmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

func TestPrefixer(t *testing.T) {
	ctx := context.Background()
	store := sinmem.NewStore()
	if err := kvtx_kvtest.TestAll(ctx, store); err != nil {
		t.Fatal(err.Error())
	}
	prefixed := NewPrefixer(store, []byte("testing-prefix/"))
	if err := kvtx_kvtest.TestAll(ctx, prefixed); err != nil {
		t.Fatal(err.Error())
	}
}
