package assembly_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/bldr/assembly"
	assembly_block "github.com/aperturerobotics/bldr/assembly/block"
	bridge_cresolve "github.com/aperturerobotics/bldr/assembly/bridge/cresolve"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/core"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	"github.com/sirupsen/logrus"
)

const testConfigSetYAML = `
test-config:
  config:
    exampleField: "hello world"
  id: controllerbus/example/boilerplate/1
  revision: 1
`

// TestAssemblyController tests the assembly controller.
func TestAssemblyController(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	sr.AddFactory(NewFactory(b))
	sr.AddFactory(bridge_cresolve.NewFactory(b))
	sr.AddFactory(boilerplate_controller.NewFactory(b))

	// run configset controller
	configsetCtrl, err := configset_controller.NewController(le, b)
	if err != nil {
		t.Fatal(err.Error())
	}

	configsetRel, err := b.AddController(ctx, configsetCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer configsetRel()

	// run assembly controller
	_, _, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&Config{
			DisablePartialSuccess: true,
		}),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ctrlRef.Release()

	ctrlExecProto, err := controller_exec.ExecControllerProtoFromConfigSet(configset.ConfigSet{
		"test-config-2": configset.NewControllerConfig(1, &boilerplate_controller.Config{
			ExampleField: "hello world #2",
		}),
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	conf := bridge_cresolve.NewControllerConfig("")
	asm := &assembly_block.Assembly{
		ControllerExec: &controller_exec.ExecControllerRequest{
			ConfigSetYaml: testConfigSetYAML,
		},
		SubAssemblies: []*assembly_block.SubAssembly{{
			Id: "test-subassembly-1",
			DirectiveBridges: []*assembly_block.DirectiveBridge{{
				BridgeToParent:   true,
				ControllerConfig: conf,
			}},
			Assemblies: []*assembly_block.Assembly{{
				ControllerExec: ctrlExecProto,
			}},
		}},
	}
	asmCs := assembly_block.NewAssemblyCursor(asm, nil)
	dir := assembly.NewApplyAssembly(asmCs)

	av, avRef, err := bus.ExecOneOff(ctx, b, dir, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer avRef.Release()

	val := av.GetValue().(assembly.ApplyAssemblyValue)
	stateCh := make(chan assembly.State, 1)
	val.AddStateCb(func(s assembly.State) {
		select {
		case <-ctx.Done():
		case stateCh <- s:
		}
	})

WaitLoop:
	for {
		select {
		case <-ctx.Done():
			return
		case v := <-stateCh:
			if err := v.GetError(); err != nil {
				t.Fatal(err.Error())
			}
			cStat := v.GetControllerStatus()
			t.Logf("got status: %s", cStat.String())
			if cStat == controller_exec.ControllerStatus_ControllerStatus_RUNNING &&
				len(v.GetSubAssemblies()) == 1 {
				saState := v.GetSubAssemblies()[0]
				t.Logf("got subassembly status: %#v", saState.GetError())
				if serr := saState.GetError(); serr != nil {
					t.Fatal(serr.Error())
				}
				if len(saState.GetAssemblies()) == 1 &&
					saState.GetAssemblies()[0].GetControllerStatus() == controller_exec.ControllerStatus_ControllerStatus_RUNNING {
					break WaitLoop
				}
			}
		}
	}
}
