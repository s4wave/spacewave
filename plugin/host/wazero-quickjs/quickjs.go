package plugin_host_wazero_quickjs

import (
	"context"
	"encoding/base64"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	"github.com/aperturerobotics/bifrost/util/randstring"
	bifrost_rwc "github.com/aperturerobotics/bifrost/util/rwc"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	"github.com/aperturerobotics/bldr/util/wazerofs"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	quickjs "github.com/aperturerobotics/go-quickjs-wasi-reactor/wazero-quickjs"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/refcount"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/tetratelabs/wazero"
	wazero_exp_sysfs "github.com/tetratelabs/wazero/experimental/sysfs"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// ControllerID is the wazero-quickjs host controller ID.
const ControllerID = "bldr/plugin/host/wazero-quickjs"

// Controller is the plugin host controller tytpe.
type Controller = host_controller.Controller

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

var (
	// BootFsMount is the path we mount the quickjs vm entrypoint.
	BootFsMount = "/boot"

	// DistFsMount is the path we mount the dist fs within the vm.
	DistFsMount = "/dist"

	// AssetsFsMount is the path we mount the assets fs within the vm.
	AssetsFsMount = "/assets"

	// DevFsMount is the path we mount the system device files within the vm.
	DevFsMount = "/dev"

	// BDirMount is the path we mount the /b/ tree within the vm
	BDirMount = "/b"

	// BDirWebPkgsMount is the path within BDir we mount the web pkgs within the vm.
	BDirWebPkgsMount = "pkg"
)

// WazeroQuickJsHost implements the plugin host with QuickJS running as WASI in Wazero.
type WazeroQuickJsHost struct {
	// b is the bus
	b bus.Bus
	// le is the logger
	le *logrus.Entry
	// pluginPlatformID is the plugin platform to use
	pluginPlatformID string
	// quickjsVmRc is a routine to compile quickjs wasi
	// this is released after a timeout to free up memory if no plugins are running.
	// shared between plugin instances to save memory and speed up startup time.
	quickjsVmRc *refcount.RefCount[*quickjsVm]
}

// quickjsVm contains the shared compilation cache for quickjs instances.
// Each plugin instance gets its own wazero.Runtime to avoid module name
// collisions, but they share compiled native code via the cache.
type quickjsVm struct {
	cache wazero.CompilationCache
}

// NewWazeroQuickJsHost constructs a new WazeroQuickJsHost.
func NewWazeroQuickJsHost(b bus.Bus, le *logrus.Entry) (*WazeroQuickJsHost, error) {
	// determine the platform id for the host
	platformID := bldr_platform.NewJsPlatform().GetPlatformID()
	h := &WazeroQuickJsHost{
		b:                b,
		le:               le,
		pluginPlatformID: platformID,
	}
	h.quickjsVmRc = refcount.NewRefCount(nil, false, nil, nil, h.resolveQuickjsVm)
	return h, nil
}

// resolveQuickjsVm resolves the shared compilation cache for quickjs plugins.
func (h *WazeroQuickJsHost) resolveQuickjsVm(ctx context.Context, released func()) (*quickjsVm, func(), error) {
	cache := wazero.NewCompilationCache()
	rel := func() { _ = cache.Close(ctx) }

	// Pre-warm the cache by compiling once.
	runtimeConfig := wazero.NewRuntimeConfig().
		WithCompilationCache(cache).
		WithCloseOnContextDone(true)
	r := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)
	_, err := wasi_snapshot_preview1.Instantiate(ctx, r)
	if err != nil {
		_ = r.Close(ctx)
		rel()
		return nil, nil, err
	}
	_, err = quickjs.CompileQuickJS(ctx, r)
	_ = r.Close(ctx)
	if err != nil {
		rel()
		return nil, nil, err
	}

	return &quickjsVm{cache: cache}, rel, nil
}

// newPluginRuntime creates a per-plugin wazero.Runtime with the shared compilation cache.
func (h *WazeroQuickJsHost) newPluginRuntime(ctx context.Context, cache wazero.CompilationCache) (wazero.Runtime, wazero.CompiledModule, error) {
	runtimeConfig := wazero.NewRuntimeConfig().
		WithCompilationCache(cache).
		WithCloseOnContextDone(true)
	r := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)
	_, err := wasi_snapshot_preview1.Instantiate(ctx, r)
	if err != nil {
		_ = r.Close(ctx)
		return nil, nil, err
	}
	mod, err := quickjs.CompileQuickJS(ctx, r)
	if err != nil {
		_ = r.Close(ctx)
		return nil, nil, err
	}
	return r, mod, nil
}

// NewWazeroQuickJsHostController constructs the WazeroQuickJsHost and PluginHost controller.
func NewWazeroQuickJsHostController(
	le *logrus.Entry,
	b bus.Bus,
	c *Config,
) (*host_controller.Controller, *WazeroQuickJsHost, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	pluginHost, err := NewWazeroQuickJsHost(b, le)
	if err != nil {
		return nil, nil, err
	}
	hctrl := host_controller.NewController(
		le,
		b,
		controller.NewInfo(ControllerID, Version, "plugin host with QuickJS running as WASI in Wazero"),
		pluginHost,
	)
	return hctrl, pluginHost, nil
}

// Execute is a stub as the wazero host does not need a global management goroutine.
func (h *WazeroQuickJsHost) Execute(ctx context.Context) error {
	_ = h.quickjsVmRc.SetContext(ctx)
	return nil
}

// GetPlatformId returns the plugin platform ID for this host.
func (h *WazeroQuickJsHost) GetPlatformId() string {
	return h.pluginPlatformID
}

// ListPlugins lists the set of initialized plugins.
func (h *WazeroQuickJsHost) ListPlugins(ctx context.Context) ([]string, error) {
	// TODO list stored plugins or temporary storage
	// the plugin host will call Delete for any unrecognized
	return nil, nil
}

// ExecutePlugin executes the plugin with the given ID.
// If the plugin was already initialized, existing state can be reused.
// The plugin should be stopped if/when the function exits.
// Return ErrPluginUninitialized if the plugin was not ready.
// Should expect to be called only once (at a time) for a plugin ID.
// pluginDist contains the plugin distribution files (binaries and assets).
func (h *WazeroQuickJsHost) ExecutePlugin(
	rctx context.Context,
	pluginID, entrypoint string,
	pluginDist, pluginAssets *unixfs.FSHandle,
	hostMux srpc.Mux,
	rpcInit plugin_host.PluginRpcInitCb,
) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// restrict to .mjs and .js only
	if !strings.HasSuffix(entrypoint, ".mjs") && !strings.HasSuffix(entrypoint, ".js") {
		return errors.Errorf("entrypoint must have a .mjs or .js extension: %q", entrypoint)
	}

	// double-check the entrypoint exists and is executable
	entrypoint = filepath.Clean(entrypoint)
	entrypointHandle, _, err := pluginDist.LookupPath(ctx, entrypoint)
	if err != nil {
		return errors.Wrapf(err, "entrypoint at %s", entrypoint)
	}

	entrypointFi, err := entrypointHandle.GetFileInfo(ctx)
	entrypointHandle.Release()
	if err != nil {
		return errors.Wrap(err, "entrypoint")
	}

	entrypointFiMode := entrypointFi.Mode()
	if !entrypointFiMode.IsRegular() {
		return errors.Errorf("entrypoint must be an executable regular file: %s", entrypointFiMode.String())
	}

	// start loading the quickjs wasm module (kick it off)
	loadVmRef := h.quickjsVmRc.AddRef(nil)
	defer loadVmRef.Release()

	// create unique plugin instance id
	pluginInstanceID := randstring.RandomIdentifier(4)
	pluginStartInfo := plugin.NewPluginStartInfo(pluginInstanceID, pluginID)
	pluginStartInfoJson, err := pluginStartInfo.MarshalJSON()
	if err != nil {
		return err
	}
	pluginStartInfoB64 := base64.StdEncoding.EncodeToString(pluginStartInfoJson)

	// Mount the RPC handler to the bus.
	baseControllerID := ControllerID + "/" + pluginID
	rpcServiceControllerID := baseControllerID + "/rpc-host"
	var hostInvoker srpc.Invoker = hostMux
	rpcServiceCtrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo(rpcServiceControllerID, Version, "rpc host for plugin"),
		func(ctx context.Context, released func()) (srpc.Invoker, func(), error) {
			return hostInvoker, nil, nil
		},
		nil,
		false,
		nil,
		nil,
		regexp.MustCompile("^"+regexp.QuoteMeta("wazero-quickjs/"+pluginInstanceID)+"$"),
	)
	relRpcServiceCtrl, err := h.b.AddController(ctx, rpcServiceCtrl, nil)
	if err != nil {
		return err
	}
	defer relRpcServiceCtrl()

	le := h.le.WithFields(logrus.Fields{
		"plugin-instance-id": pluginInstanceID,
		"plugin-id":          pluginID,
	})
	le.Debug("starting wazero quickjs plugin instance")

	// this restarts if the quickjs vm is reloaded or unloaded
	return h.quickjsVmRc.Access(ctx, func(ctx context.Context, val *quickjsVm) error {
		// Create a per-plugin runtime with the shared compilation cache.
		// Each plugin gets its own runtime to avoid wazero module name collisions
		// (the QuickJS library forces the module name to "qjs-wasi.wasm").
		r, compiled, err := h.newPluginRuntime(ctx, val.cache)
		if err != nil {
			return errors.Wrap(err, "create plugin runtime")
		}
		defer r.Close(ctx)

		// construct a filesystem with the plugin dist fs at /dist
		// this makes /dist read-only which is what we want.
		// wazeroFs := wazerofs.NewFS(ctx, pluginDist, nil)
		pluginDistIofs := unixfs_iofs.NewFS(ctx, pluginDist)
		pluginAssetsIoFs := unixfs_iofs.NewFS(ctx, pluginAssets)

		// construct the fs config
		fsConfig := wazero.NewFSConfig().
			WithFSMount(pluginDistIofs, DistFsMount).
			WithFSMount(pluginAssetsIoFs, AssetsFsMount)

		// script is within dist
		scriptPath := path.Join(DistFsMount, entrypoint)

		// mount the boot js file to /boot/plugin-quickjs.esm.js
		fsConfig = fsConfig.WithFSMount(PluginQuickjsBoot, BootFsMount)

		// Create the stdin buffer.
		stdinBuf := &wazerofs.StdinBuffer{}
		defer stdinBuf.Close()

		// Create the output pipe + buffer.
		localRead, remoteWrite := io.Pipe()

		// create read/write/closer for local I/O
		// we will read from the localRead end of the pipe, and write to the stdinBuf.
		localRwc := bifrost_rwc.NewReadWriteCloser(localRead, stdinBuf)

		// initialize the yamux client for talking to the plugin.
		yamuxConf := srpc.NewYamuxConfig()
		yamuxConf.EnableKeepAlive = false
		yamuxConf.MaxMessageSize = 32 * 1024 // use a 32kb buffer for stdin

		// NOTE: we could probably also disable rtt measurement.
		muxedConn, err := srpc.NewMuxedConnWithRwc(ctx, localRwc, true, yamuxConf)
		if err != nil {
			return err
		}
		defer muxedConn.Close()

		// NOTE stdin is currently the only fd which implements Poll.
		// all other fds will call and block on read() (blocking I/O only).
		// https://github.com/tetratelabs/wazero/issues/1500#issuecomment-3041125375
		// we therefore must use stdin for our async i/o input, and a file at /dev/out for output
		// force WR_ONLY on that file.
		//
		// See prototype under prototypes/js-wazero-quickjs/nonblock/
		devFS := newDevFS(remoteWrite)
		fsConfig = fsConfig.(wazero_exp_sysfs.FSConfig).WithSysFSMount(devFS, DevFsMount)

		// Initialize the rpc client for calling the plugin.
		openStreamFn := srpc.NewOpenStreamWithMuxedConn(muxedConn)
		pluginRpcClient := srpc.NewClient(openStreamFn)
		if err := rpcInit(pluginRpcClient); err != nil {
			return err
		}

		// Execute the muxed conn on the server side (accept incoming streams).
		var acceptStreamErr atomic.Pointer[error]
		srv := srpc.NewServer(hostMux)
		go func() {
			err := srv.AcceptMuxedConn(ctx, muxedConn)
			if err != nil && ctx.Err() == nil {
				acceptStreamErr.Store(&err)
				ctxCancel()
			}
		}()

		// Log to the logger instead of directly to stderr.
		debugWriter := le.WriterLevel(logrus.DebugLevel)
		moduleConfig := wazero.NewModuleConfig().
			WithStdin(stdinBuf).
			WithStdout(debugWriter).
			WithStderr(debugWriter).
			WithFS(pluginDistIofs).
			WithFSConfig(fsConfig).
			WithSysNanosleep().
			WithSysNanotime().
			WithSysWalltime().
			WithEnv("BLDR_SCRIPT_PATH", scriptPath).
			WithEnv("BLDR_PLUGIN_START_INFO", pluginStartInfoB64)

		// Create a QuickJS instance using the pre-compiled module.
		// This uses the reactor model which doesn't block in _start().
		qjs, err := quickjs.NewQuickJSWithModule(ctx, r, compiled, moduleConfig)
		if err != nil {
			return errors.Wrap(err, "create quickjs instance")
		}
		defer qjs.Close(ctx)

		// Initialize QuickJS with CLI args to load the boot harness.
		// The boot harness reads BLDR_SCRIPT_PATH and BLDR_PLUGIN_START_INFO from env.
		bootScript := path.Join(BootFsMount, "plugin-quickjs.esm.js")
		if err := qjs.Init(ctx, []string{"qjs", "--std", bootScript}); err != nil {
			return errors.Wrap(err, "init quickjs")
		}

		// Run the event loop with stdin polling.
		// Unlike RunLoop which exits on idle, we need to wait for stdin data
		// when idle since the plugin is waiting for RPC messages.
		err = runLoopWithStdin(ctx, qjs, stdinBuf)
		_ = remoteWrite.Close()

		if errPtr := acceptStreamErr.Load(); errPtr != nil {
			return *errPtr
		}
		if ctx.Err() != nil {
			return context.Canceled
		}
		if err != nil {
			return errors.Wrap(err, "quickjs event loop")
		}

		return nil
	})
}

// runLoopWithStdin runs the QuickJS event loop with stdin polling.
// When the event loop becomes idle or waiting for a timer, it also monitors stdin.
// This keeps the plugin alive waiting for RPC messages.
//
// The reactor model's qjs_loop_once() only handles timers and microtasks - it does
// NOT poll for I/O. We use qjs_poll_io() to invoke os.setReadHandler callbacks when
// stdin has data available. This is more efficient than the command model because
// the host knows exactly when I/O is ready, avoiding unnecessary poll() syscalls.
func runLoopWithStdin(ctx context.Context, qjs *quickjs.QuickJS, stdinBuf *wazerofs.StdinBuffer) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		result, err := qjs.LoopOnce(ctx)
		if err != nil {
			return err
		}

		switch {
		case result == quickjs.LoopError:
			return errors.New("JavaScript error occurred")
		case result == 0:
			// More microtasks pending, continue immediately
			continue
		case result > 0:
			// Timer pending - wait for timer, stdin data, or context cancellation
			ready, waitCh := stdinBuf.CheckReady()
			if ready {
				// Data available - poll I/O to invoke read handlers
				if _, err := qjs.PollIO(ctx, 0); err != nil {
					return err
				}
				continue
			}
			timerDur := time.Duration(result.NextTimerMs()) * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-waitCh:
				// Stdin data available - poll I/O to invoke read handlers
				if _, err := qjs.PollIO(ctx, 0); err != nil {
					return err
				}
				continue
			case <-time.After(timerDur):
				continue
			}
		case result == quickjs.LoopIdle:
			// Idle - wait for stdin data or context cancellation
			ready, waitCh := stdinBuf.CheckReady()
			if ready {
				// Data available - poll I/O to invoke read handlers
				if _, err := qjs.PollIO(ctx, 0); err != nil {
					return err
				}
				continue
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-waitCh:
				// Stdin data available - poll I/O to invoke read handlers
				if _, err := qjs.PollIO(ctx, 0); err != nil {
					return err
				}
				continue
			}
		}
	}
}

// DeletePlugin clears cached plugin data for the given plugin ID.
func (h *WazeroQuickJsHost) DeletePlugin(ctx context.Context, pluginID string) error {
	// TODO remove caches or local storage?
	return nil
}

// _ is a type assertion
var _ plugin_host.PluginHost = (*WazeroQuickJsHost)(nil)
