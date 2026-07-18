package pass_tx

import (
	"context"
	"testing"

	forge_pass "github.com/s4wave/spacewave/forge/pass"
	"github.com/s4wave/spacewave/net/peer"
)

func TestTxStartAdoptsRunningPass(t *testing.T) {
	root := &forge_pass.Pass{
		PassState: forge_pass.State_PassState_RUNNING,
	}

	err := NewTxStart("pass", nil, false).GetTxStart().ExecuteTx(
		context.Background(), nil, peer.ID("peer"), "pass", nil, root,
	)
	if err != nil {
		t.Fatalf("adopt running pass: %v", err)
	}
	if got := root.GetPassState(); got != forge_pass.State_PassState_RUNNING {
		t.Fatalf("pass state changed to %s", got)
	}
}
