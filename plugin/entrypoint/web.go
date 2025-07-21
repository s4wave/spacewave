//go:build js
// +build js

package plugin_entrypoint

import (
	"context"
	"io"
	"os"
	"syscall/js"
	"time"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	web_runtime_wasm "github.com/aperturerobotics/bldr/web/runtime/wasm"
	"github.com/aperturerobotics/starpc/srpc"
	fetch "github.com/aperturerobotics/util/js/fetch"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the entrypoint version
var Version = semver.MustParse("0.0.1")

// Main runs the default main entrypoint for a plugin.
func Main(
	pluginStartInfoJsonB64,
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

		pluginStartInfo, err := UnmarshalPluginStartInfo(pluginStartInfoJsonB64)
		if err != nil {
			return err
		}

		pluginMeta, err := UnmarshalPluginMeta(pluginMetaB58)
		if err != nil {
			return err
		}

		err = Run(ctx, le, pluginStartInfo, pluginMeta, addFactoryFuncs, configSetFuncs, pluginIo)
		if err != context.Canceled {
			return err
		}

		return nil
	}(); err != nil {
		le.WithError(err).Error("exiting with fatal error")
		ctxCancel()
		<-time.After(time.Millisecond * 100)
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

// readFile reads from a file using fetch, resolving the path relative to the module URL.
// It resolves the filePath relative to the current module's URL using JavaScript's URL constructor.
func readFile(filePath string) ([]byte, error) {
	// use js to determine the full path to filePath based on import.meta.url
	// this is because the path to the shw.mjs is different than the path to the plugin entrypoint .mjs
	// we need to join filePath with the path to /b/pd/{plugin-id}/
	//
	// Construct the URL: new URL(filePath, BLDR_BASE_URL) and get the pathname.
	// BLDR_BASE_URL should be set to the equivalent of import.meta.url for the main module.
	// Get BLDR_BASE_URL from the global scope.
	baseUrlVal := js.Global().Get("BLDR_BASE_URL")
	// Check if BLDR_BASE_URL is defined and not empty.
	if !baseUrlVal.Truthy() {
		return nil, errors.New("BLDR_BASE_URL is not defined")
	}

	// Construct the URL object. This call might panic if the arguments are invalid.
	resolvedPath := js.Global().Get("URL").New(filePath, baseUrlVal).Get("pathname").String()

	// Fetch the resolved path
	resp, err := fetch.Fetch(resolvedPath, &fetch.Opts{
		Method: fetch.MethodGet,
		CommonOpts: fetch.CommonOpts{
			Cache: "no-store",
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "fetching resolved path: %s", resolvedPath)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.Errorf("request returned status %v: %s", resp.Status, resolvedPath)
	}
	return io.ReadAll(resp.Body)
}
