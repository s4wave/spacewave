package target_json

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/core"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	forge_target "github.com/aperturerobotics/forge/target"
	mock "github.com/aperturerobotics/forge/target/mock"
	"github.com/sirupsen/logrus"
)

// SampleTargetYAML is a example yaml config.
const SampleTargetYAML = mock.TargetYAML

func TestTarget_YAML(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tgt := &Target{}
	err := tgt.UnmarshalYAML([]byte(SampleTargetYAML))
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(tgt.underlying.GetOutputs()) != 1 {
		t.Fail()
	}
	if tgt.underlying.GetOutputs()[0].GetOutputType() != forge_target.OutputType_OutputType_EXEC {
		t.Fail()
	}
	t.Logf("parsed underlying: %s", tgt.underlying.String())
	t.Logf("parsed configset_json separately: %s", tgt.execControllerConfig.Id)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	sr.AddFactory(boilerplate_controller.NewFactory(b))

	// test resolve proto
	tgtProto, err := tgt.ResolveProto(ctx, b)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(tgtProto.GetExec().GetController().GetConfig()) == 0 {
		t.Fail()
	}
	if tgtProto.GetExec().GetController().GetId() != tgt.execControllerConfig.Id {
		t.Fail()
	}

	cc, err := tgtProto.GetExec().GetController().Resolve(ctx, b)
	if err != nil {
		t.Fatal(err.Error())
	}
	if cc.GetConfig().GetConfigID() != tgtProto.Exec.GetController().GetId() {
		t.Fail()
	}
	if cc.GetConfig().(*boilerplate_controller.Config).GetExampleField() != "Hello world" {
		t.Fail()
	}
	t.Logf("constructed config successfully: %#v", cc.GetConfig())
}
