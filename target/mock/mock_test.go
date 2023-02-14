package forge_target_mock

import (
	"context"
	"testing"

	forge_lib_kvtx "github.com/aperturerobotics/forge/lib/kvtx"
	"github.com/aperturerobotics/forge/testbed"
)

func TestTarget_YAML(t *testing.T) {
	ctx := context.Background()
	tb, _ := testbed.Default(ctx)
	b := tb.Bus
	tb.StaticResolver.AddFactory(forge_lib_kvtx.NewFactory(b))

	tgt, err := ResolveMockTarget(ctx, b)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(tgt.GetExec().GetController().GetConfig()) == 0 {
		t.Fail()
	}
	if tgt.GetExec().GetController().GetId() != "forge/lib/kvtx" {
		t.Fail()
	}

	cc, err := tgt.GetExec().GetController().Resolve(ctx, b)
	if err != nil {
		t.Fatal(err.Error())
	}
	if cc.GetConfig().GetConfigID() != tgt.Exec.GetController().GetId() {
		t.Fail()
	}
	if len(cc.GetConfig().(*forge_lib_kvtx.Config).GetOps()) != 5 {
		t.Fail()
	}
	t.Logf("constructed config successfully: %#v", cc.GetConfig())
}
