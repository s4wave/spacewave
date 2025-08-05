//go:build !js

package bldr_plugin_compiler_js_test

import (
	"context"
	"testing"

	plugin_host_wazero_quickjs "github.com/aperturerobotics/bldr/plugin/host/wazero-quickjs"
	"github.com/aperturerobotics/bldr/testbed"
	"github.com/sirupsen/logrus"
)

func TestPluginCompilerJs(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.BuildTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	b, sr := tb.GetBus(), tb.GetStaticResolver()
	sr.AddFactory(plugin_host_wazero_quickjs.NewFactory(b))
}
