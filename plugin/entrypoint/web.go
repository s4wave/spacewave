//go:build js
// +build js

package plugin_entrypoint

import (
	"context"
	"os"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	fetch "github.com/aperturerobotics/bldr/util/wasm-fetch"
	web_runtime_wasm "github.com/aperturerobotics/bldr/web/runtime/wasm"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the entrypoint version
var Version = semver.MustParse("0.0.1")

// Main runs the default main entrypoint for a plugin.
func Main(
	pluginStartInfoB58,
	pluginMetaB58 string,
	logLevel logrus.Level,
	addFactoryFuncs []AddFactoryFunc,
	configSetFuncs []BuildConfigSetFunc,
) {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	})
	log.SetLevel(logLevel)
	le := logrus.NewEntry(log)

	// There is no os.Interrupt on js.
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	if err := func() error {
		pluginIo, err := web_runtime_wasm.GlobalWasmPluginIo()
		if err != nil {
			return err
		}

		pluginStartInfo, pluginMeta, err := UnmarshalPluginStartInfo(pluginStartInfoB58, pluginMetaB58)
		if err != nil {
			return err
		}

		err = Run(ctx, le, pluginStartInfo, pluginMeta, addFactoryFuncs, configSetFuncs, pluginIo)
		if err != context.Canceled {
			return err
		}

		return nil
	}(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

// Run runs the plugin entrypoint.
func Run(
	ctx context.Context,
	le *logrus.Entry,
	pluginStartInfo *bldr_plugin.PluginStartInfo,
	pluginMeta *bldr_plugin.PluginMeta,
	addFactoryFuncs []AddFactoryFunc,
	configSetFuncs []BuildConfigSetFunc,
	pluginIo *web_runtime_wasm.WasmPluginIo,
) error {
	if err := pluginStartInfo.Validate(); err != nil {
		return err
	}

	instanceID := pluginStartInfo.GetInstanceId()
	_ = instanceID

	// dial outgoing streams and accept incoming streams
	rpcClient := pluginIo.BuildClient()
	acceptRpcStreams := func(ctx context.Context, srv *srpc.Server) error {
		pluginIo.SetAcceptStreams(ctx, srv.GetInvoker())
		return nil
	}

	return ExecutePlugin(
		ctx,
		le,
		pluginMeta,
		addFactoryFuncs,
		configSetFuncs,
		rpcClient,
		acceptRpcStreams,
	)
}

// readFile reads from a file using fetch.
func readFile(filePath string) ([]byte, error) {
	resp, err := fetch.Fetch(filePath, &fetch.Opts{
		Method: fetch.MethodGet,
		Cache:  "no-store",
	})
	if err != nil {
		return nil, err
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return nil, errors.Errorf("request returned status %v: %s", resp.Status, filePath)
	}
	return resp.Body, nil
}
