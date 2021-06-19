package execution_controller_testing

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	execution_mock "github.com/aperturerobotics/forge/execution/mock"
	forge_target "github.com/aperturerobotics/forge/target"
	"github.com/aperturerobotics/forge/target/json"
	target_mock "github.com/aperturerobotics/forge/target/mock"
	"github.com/aperturerobotics/hydra/core"
	"github.com/sirupsen/logrus"
)

// TestExecutionController_Simple tests basic mechanics of the execution controller.
func TestExecutionController_Simple(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// add the boilerplate controller factory
	// referenced in the Target below
	sr.AddFactory(boilerplate_controller.NewFactory(b))

	execConf := &boilerplate_controller.Config{
		ExampleField: "Hello world",
	}
	execCtrlConf, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, execConf))
	if err != nil {
		t.Fatal(err.Error())
	}
	forgeTarget := &forge_target.Target{
		Exec: &forge_target.Exec{
			Controller: execCtrlConf,
		},
	}
	forgeTargetJSON := &target_json.Target{}
	err = forgeTargetJSON.SetTarget(ctx, b, forgeTarget)
	if err != nil {
		t.Fatal(err.Error())
	}
	yamlData, err := forgeTargetJSON.MarshalYAML()
	if err != nil {
		t.Fatal(err.Error())
	}

	forgeTargetYaml := &target_json.Target{}
	err = forgeTargetYaml.UnmarshalYAML(yamlData)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = execution_mock.RunTargetInTestbed(ctx, le, forgeTargetYaml)
	if err != nil {
		t.Fatal(err.Error())
	}
}

// TestExecutionController_FromYAML tests basic mechanics of the execution controller, from a yaml config.
func TestExecutionController_FromYAML(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// add the boilerplate controller factory
	// referenced in the Target below
	sr.AddFactory(boilerplate_controller.NewFactory(b))

	// initial unmarshal yaml pass
	jsonTarget := &target_json.Target{}
	err = jsonTarget.UnmarshalYAML([]byte(target_mock.TargetYAML))
	if err != nil {
		t.Fatal(err.Error())
	}

	err = execution_mock.RunTargetInTestbed(ctx, le, jsonTarget)
	if err != nil {
		t.Fatal(err.Error())
	}
}
