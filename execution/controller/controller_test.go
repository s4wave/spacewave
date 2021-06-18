package execution_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/bifrost/keypem"
	peer_controller "github.com/aperturerobotics/bifrost/peer/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
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

	mockHandler := NewMockHandler(nil)
	peerCtrl, err := mountTestPeer(ctx, le, b)
	if err != nil {
		t.Fatal(err.Error())
	}
	peerID := peerCtrl.GetPeerID()

	execConf := &boilerplate_controller.Config{
		ExampleField: "Hello world",
	}
	execCtrlConf, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, execConf))
	if err != nil {
		t.Fatal(err.Error())
	}
	conf := &Config{
		PeerId: peerID.Pretty(),
		Target: &forge_target.Target{
			Exec: &forge_target.Exec{
				Controller: execCtrlConf,
			},
		},
		ResolveControllerConfigTimeout: "5s",
		AllowNonExecController:         true,
	}
	ctrl := NewController(le, b, conf, mockHandler)
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()
	ctrlErr := b.ExecuteController(subCtx, ctrl)
	if ctrlErr != nil {
		// expect successful exit
		t.Fatal(ctrlErr.Error())
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

	mockHandler := NewMockHandler(nil)

	// initial unmarshal yaml pass
	jsonTarget := &target_json.Target{}
	err = jsonTarget.UnmarshalYAML([]byte(target_mock.TargetYAML))
	if err != nil {
		t.Fatal(err.Error())
	}

	// convert the yaml target into protobuf
	resolvedTarget, err := jsonTarget.ResolveProto(ctx, b)
	if err != nil {
		t.Fatal(err.Error())
	}

	peerCtrl, err := mountTestPeer(ctx, le, b)
	if err != nil {
		t.Fatal(err.Error())
	}
	peerID := peerCtrl.GetPeerID()

	conf := &Config{
		PeerId:                         peerID.Pretty(),
		Target:                         resolvedTarget,
		ResolveControllerConfigTimeout: "5s",
		AllowNonExecController:         true,
	}
	ctrl := NewController(le, b, conf, mockHandler)
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()
	ctrlErr := b.ExecuteController(subCtx, ctrl)
	if ctrlErr != nil {
		// expect successful exit
		t.Fatal(ctrlErr.Error())
	}
}

// mountTestPeer starts a test peer executing on the bus.
func mountTestPeer(ctx context.Context, le *logrus.Entry, b bus.Bus) (*peer_controller.Controller, error) {
	privKey, _, err := keypem.GeneratePrivKey()
	if err != nil {
		return nil, err
	}
	peerCtrl, err := peer_controller.NewController(le, privKey)
	if err != nil {
		return nil, err
	}
	go b.ExecuteController(ctx, peerCtrl)
	return peerCtrl, nil
}
