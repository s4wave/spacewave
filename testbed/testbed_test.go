package testbed

import (
	"context"
	"testing"

	plugin_host_scheduler "github.com/aperturerobotics/bldr/plugin/host/scheduler"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/sirupsen/logrus"
)

func TestTestbed(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := BuildTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	b, sr := tb.GetBus(), tb.GetStaticResolver()
	sr.AddFactory(plugin_host_scheduler.NewFactory(b))

	// verify the world started ok
	eng := tb.GetWorldEngine()
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	tx.Discard()

	// load the plugin scheduler
	ctrl, _, ctrlRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_scheduler.Controller](
		ctx,
		tb.GetBus(),
		resolver.NewLoadControllerWithConfig(plugin_host_scheduler.NewConfig(
			tb.GetWorldEngineID(),
			tb.GetPluginHostObjKey(),
			tb.GetVolumeInfo().GetVolumeId(),
			tb.GetVolumeInfo().GetPeerId(),
			true,
			false,
			false,
		)),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ctrlRef.Release()
	_ = ctrl
}
