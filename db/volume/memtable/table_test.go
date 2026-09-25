package memtable

import (
	"context"
	"testing"

	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
)

// TestKV runs the kvtx conformance tests on a table that persists nothing.
func TestKV(t *testing.T) {
	table := New(func(context.Context, Snapshot, []Op, bool) error { return nil })
	if err := kvtx_kvtest.TestAll(t.Context(), table); err != nil {
		t.Fatal(err)
	}
}
