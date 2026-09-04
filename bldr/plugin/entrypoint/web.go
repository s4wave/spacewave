//go:build js

package plugin_entrypoint

import (
	"context"
	"io"
	"os"
	"syscall/js"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	fetch "github.com/aperturerobotics/util/js/fetch"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/sirupsen/logrus"
)

// pluginTransport abstracts the plugin-side SRPC transport used by the web
// entrypoint. OpenStream dials the plugin host; SetAcceptStreams registers the
// server-side invoker so the host can dial streams into this plugin.
type pluginTransport interface {
	// OpenStream opens a stream to the plugin host.
	OpenStream(ctx context.Context, msgHandler srpc.PacketDataHandler, closeHandler srpc.CloseHandler) (srpc.PacketWriter, error)

	// SetAcceptStreams registers the invoker used to accept incoming streams
	// from the plugin host.
	SetAcceptStreams(ctx context.Context, invoker srpc.Invoker)
}

// Version is the entrypoint version
var Version = controller.MustParseVersion("0.0.1")

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
		pluginIo, err := newPluginTransport()
		if err != nil {
			return err
		}

		if pluginStartInfoJsonB64 == "" {
			startInfoVal := js.Global().Get("BLDR_PLUGIN_START_INFO")
			if startInfoVal.Truthy() {
				pluginStartInfoJsonB64 = startInfoVal.String()
			}
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
		if !isExpectedPluginEntrypointError(err) {
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

// Run runs the plugin entrypoint over the given transport.
func Run(
	ctx context.Context,
	le *logrus.Entry,
	pluginStartInfo *bldr_plugin.PluginStartInfo,
	pluginMeta *bldr_plugin.PluginMeta,
	addFactoryFuncs []AddFactoryFunc,
	configSetFuncs []BuildConfigSetFunc,
	pluginIo pluginTransport,
) error {
	if err := pluginStartInfo.Validate(); err != nil {
		return err
	}

	// Dial outgoing streams and accept incoming streams.
	rpcClient := srpc.NewClient(pluginIo.OpenStream)
	acceptRpcStreams := func(ctx context.Context, srv *srpc.Server, ready func()) error {
		pluginIo.SetAcceptStreams(ctx, srv.GetInvoker())
		ready()
		return nil
	}

	return ExecutePluginEntrypoint(
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
