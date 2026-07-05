package kvtx_rpc_client

import (
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
)

// TestOpsNoBatch asserts the RPC client ops deliberately does not implement
// kvtx.BatchTxOps. The remote store is reachable only through the generated
// KvtxOps RPC methods, which have no batch KeyData call, so a GetBatch here
// could not collapse the per-key round trips the batch seam removes. kvtx.GetBatch
// must therefore fall back to serial Get calls for the remote store. Adding a
// batch RPC method and implementing GetBatch is a wire-protocol change; update
// this expectation together with that protocol.
func TestOpsNoBatch(t *testing.T) {
	var ops kvtx.TxOps = NewOps(nil, nil)
	if _, ok := ops.(kvtx.BatchTxOps); ok {
		t.Fatal("rpc client Ops must not implement kvtx.BatchTxOps without a batch RPC method")
	}
}
