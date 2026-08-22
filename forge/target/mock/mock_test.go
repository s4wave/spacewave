package forge_target_mock

import (
	"context"
	"testing"

	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	"github.com/s4wave/spacewave/forge/testbed"
)

func TestTarget_YAML(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	b := tb.Bus
	tb.StaticResolver.AddFactory(forge_lib_kvtx.NewFactory(b))

	tgt, err := ResolveMockTarget(ctx, b)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(tgt.GetExec().GetController().GetConfig()) == 0 {
		t.Fatal("expected non-empty controller config")
	}
	if id := tgt.GetExec().GetController().GetId(); id != "forge/lib/kvtx" {
		t.Fatalf("unexpected controller id: %q", id)
	}

	cc, err := tgt.GetExec().GetController().Resolve(ctx, b)
	if err != nil {
		t.Fatal(err.Error())
	}
	if cc.GetConfig().GetConfigID() != tgt.Exec.GetController().GetId() {
		t.Fatalf(
			"config id mismatch: %q != %q",
			cc.GetConfig().GetConfigID(),
			tgt.Exec.GetController().GetId(),
		)
	}
	if nops := len(cc.GetConfig().(*forge_lib_kvtx.Config).GetOps()); nops != 5 {
		t.Fatalf("expected 5 ops, got %d", nops)
	}
	t.Logf("constructed config successfully: %#v", cc.GetConfig())
}
