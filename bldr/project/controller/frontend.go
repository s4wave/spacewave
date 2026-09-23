//go:build !js

package bldr_project_controller

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	frontend "github.com/s4wave/spacewave/bldr/frontend"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	js_compiler "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	vite_compiler "github.com/s4wave/spacewave/bldr/web/bundler/vite/compiler"
	web_fetch "github.com/s4wave/spacewave/bldr/web/fetch"
	web_pkg_external "github.com/s4wave/spacewave/bldr/web/pkg/external"
	"github.com/sirupsen/logrus"
)

// refreshExports is the upstream React plugin's module boundary. Every compiler
// borrows the document's bootstrap runtime so renderer and family state agree.
const refreshExports = "register, injectIntoGlobalHook, createSignatureFunctionForTransform, registerExportsForReactRefresh, validateRefreshBoundaryAndEnqueueUpdate, __hmr_import"

// FrontendService owns the project's current development environment.
// The project serializes configuration and shutdown; RPC methods run concurrently.
type FrontendService struct {
	// le records environment failures and recovery.
	le *logrus.Entry
	// bus supplies the project's compiler dependencies.
	bus bus.Bus
	// run serializes configuration changes and joins replaced environments.
	run *routine.StateRoutineContainer[*vite.DevelopmentConfig]
	// ready publishes the current compiler capability to waiting RPC calls.
	ready *promise.PromiseContainer[*frontendEnvironment]
	// active permits nonblocking validation of session-bound requests.
	active atomic.Pointer[frontendEnvironment]
}

// frontendEnvironment is a ready compiler capability, valid until ctx ends.
type frontendEnvironment struct {
	// ctx ends when this environment is replaced or released.
	ctx context.Context
	// client connects to the managed compiler process.
	client vite.SRPCViteBundlerClient
	// result identifies the compiler session and its private HTTP endpoint.
	result *vite.DevelopmentResult
	// proxy forwards module requests while preserving cancellation and queries.
	proxy *httputil.ReverseProxy
}

// newFrontendService constructs a service without starting its compiler.
func newFrontendService(le *logrus.Entry, b bus.Bus) *FrontendService {
	f := &FrontendService{le: le, bus: b, ready: promise.NewPromiseContainer[*frontendEnvironment]()}
	f.run = routine.NewStateRoutineContainerWithLoggerVT[*vite.DevelopmentConfig](le, routine.WithRetry(&backoff.Backoff{}))
	f.run.SetStateRoutine(f.execute)
	return f
}

// configure replaces the graph only when its configured inputs change.
func (f *FrontendService) configure(cc *Config) error {
	conf := &vite.DevelopmentConfig{
		RootDir:      cc.GetSourcePath(),
		DistDir:      filepath.Join(cc.GetWorkingPath(), "src"),
		CacheDir:     filepath.Join(cc.GetWorkingPath(), "frontend", "vite-cache"),
		ExternalPkgs: slices.Clone(web_pkg_external.BldrExternal),
		RoutePrefix:  cc.GetFrontendRoutePrefix(),
	}
	configured := false
	for id, manifest := range cc.GetProjectConfig().GetManifests() {
		builder := manifest.GetBuilder()
		if builder.GetId() != js_compiler.ConfigID {
			continue
		}
		js := &js_compiler.Config{}
		if err := unmarshalBuilderConfig(f.le, id, "JS", builder.GetConfig(), js); err != nil {
			return err
		}
		js.FlattenBuildTypes(bldr_manifest.BuildType_DEV)
		js.FlattenPlatformTypes(bldr_platform.NewJsPlatform())
		for _, pkg := range js.GetWebPkgs() {
			if pkg.GetExclude() {
				conf.WebPkgIds = append(conf.WebPkgIds, pkg.GetId())
			}
		}
		for _, module := range js.GetModules() {
			if module.GetKind() != js_compiler.JsModuleKind_JS_MODULE_KIND_FRONTEND {
				continue
			}
			conf.Entrypoints = append(conf.Entrypoints, path.Clean(module.GetPath()))
			paths := append(slices.Clone(js.GetViteConfigPaths()), module.GetViteConfigPaths()...)
			for i, configPath := range paths {
				paths[i] = path.Clean(configPath)
			}
			disableProjectConfig := js.GetViteDisableProjectConfig() || module.GetDisableProjectConfig()
			if configured && (!slices.Equal(conf.ConfigPaths, paths) || conf.DisableProjectConfig != disableProjectConfig) {
				return errors.New("project frontend modules must share one Vite configuration")
			}
			conf.ConfigPaths = paths
			conf.DisableProjectConfig = disableProjectConfig
			configured = true
		}
	}
	slices.Sort(conf.Entrypoints)
	conf.Entrypoints = slices.Compact(conf.Entrypoints)
	slices.Sort(conf.WebPkgIds)
	conf.WebPkgIds = slices.Compact(conf.WebPkgIds)
	if len(conf.Entrypoints) == 0 {
		conf = nil
	}
	f.run.SetState(conf)
	return nil
}

// execute publishes one session and joins its compiler before any replacement.
func (f *FrontendService) execute(ctx context.Context, config *vite.DevelopmentConfig) error {
	f.ready.SetPromise(nil)
	defer func() {
		f.active.Store(nil)
		f.ready.SetPromise(nil)
	}()
	compiler, err := vite_compiler.NewController(f.le, f.bus, &vite_compiler.Config{})
	if err != nil {
		f.ready.SetResult(nil, err)
		return err
	}
	defer compiler.Close()
	if err := compiler.Execute(ctx); err != nil {
		f.ready.SetResult(nil, err)
		return err
	}

	conf := config.CloneVT()
	conf.SessionId = rand.Text()
	err = compiler.RunDevelopment(ctx, conf, func(client vite.SRPCViteBundlerClient, result *vite.DevelopmentResult) {
		target, parseErr := url.Parse(result.GetPrivateUrl())
		if parseErr != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" {
			f.ready.SetResult(nil, errors.New("frontend compiler returned an invalid private address"))
			return
		}
		// The authenticated Bldr route targets Vite's private listener. Its Host
		// must name that listener, independently of the browser's public origin.
		env := &frontendEnvironment{ctx: ctx, client: client, result: result, proxy: &httputil.ReverseProxy{
			Rewrite: func(request *httputil.ProxyRequest) { request.SetURL(target) },
		}}
		env.proxy.FlushInterval = -1
		f.active.Store(env)
		f.ready.SetResult(env, nil)
	})
	if err != nil && ctx.Err() == nil {
		f.ready.SetResult(nil, err)
	}
	return err
}

// Close cancels and joins the current environment before returning.
func (f *FrontendService) Close() {
	if done, _, _ := f.run.SetStateRoutine(nil); done != nil {
		<-done
	}
}

// Watch forwards the compiler snapshot and ordered events for one session.
func (f *FrontendService) Watch(_ *frontend.WatchRequest, stream frontend.SRPCFrontend_WatchStream) error {
	if f.run.GetState() == nil {
		return stream.Send(&frontend.Event{Session: &frontend.Session{Id: "disabled", RoutePrefix: "/b/fe/disabled/"}})
	}
	env, err := f.ready.Await(stream.Context())
	if err != nil {
		return err
	}
	watch, err := env.client.WatchDevelopment(stream.Context(), &frontend.WatchRequest{})
	if err != nil {
		return err
	}
	defer watch.Close()
	for {
		event, err := watch.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(event); err != nil {
			return err
		}
	}
}

// Send rejects messages for an expired session at the native owner.
func (f *FrontendService) Send(ctx context.Context, req *frontend.SendRequest) (*frontend.SendResponse, error) {
	env := f.active.Load()
	if env == nil || env.ctx.Err() != nil || req.GetSessionId() != env.result.GetSession().GetId() {
		return nil, errors.New("frontend session expired")
	}
	return env.client.SendDevelopment(ctx, req)
}

// Fetch serves module requests using the existing streamed HTTP protocol.
func (f *FrontendService) Fetch(stream frontend.SRPCFrontend_FetchStream) error {
	return web_fetch.HandleFetch(stream, f.ServeHTTP)
}

// ServeHTTP forwards only the current same-origin module namespace.
func (f *FrontendService) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		rw.Header().Set("Allow", "GET, HEAD")
		http.Error(rw, "frontend modules are read-only", http.StatusMethodNotAllowed)
		return
	}
	env := f.active.Load()
	if env != nil && env.ctx.Err() == nil && strings.HasPrefix(req.URL.Path, "/b/fe/entrypoint/") {
		entrypoint := strings.TrimPrefix(req.URL.Path, "/b/fe/entrypoint/")
		if !slices.Contains(env.result.GetSession().GetEntrypoints(), entrypoint) {
			http.NotFound(rw, req)
			return
		}
		target := &url.URL{Path: env.result.GetSession().GetRoutePrefix() + entrypoint, RawQuery: req.URL.RawQuery}
		rw.Header().Set("Cache-Control", "no-store")
		http.Redirect(rw, req, target.String(), http.StatusTemporaryRedirect)
		return
	}
	if env == nil || env.ctx.Err() != nil || !strings.HasPrefix(req.URL.Path, env.result.GetSession().GetRoutePrefix()) {
		http.Error(rw, "frontend session expired", http.StatusGone)
		return
	}
	rw.Header().Set("Cache-Control", "no-store")
	if req.URL.Path == env.result.GetSession().GetRoutePrefix()+"@react-refresh" {
		rw.Header().Set("Content-Type", "text/javascript")
		_, _ = io.WriteString(rw, "const runtime = globalThis.__bldrReactRefresh;\n"+
			"if (!runtime) throw new Error('Live React editing requires a Bldr development client');\n"+
			"export const {"+refreshExports+"} = runtime; export default runtime;\n")
		return
	}
	env.proxy.ServeHTTP(rw, req)
}

// ServeBootstrap establishes React Refresh before the canonical renderer loads.
// Only compiler runtime code uses this route; application modules use Fetch.
func (f *FrontendService) ServeBootstrap(rw http.ResponseWriter, req *http.Request, entrypoint string) {
	if f.run.GetState() == nil {
		rw.Header().Set("Content-Type", "text/javascript")
		rw.Header().Set("Cache-Control", "no-store")
		entry := strconv.Quote("/" + strings.TrimPrefix(entrypoint, "/"))
		_, _ = io.WriteString(rw, "await import("+entry+");\n")
		return
	}
	env, err := f.ready.Await(req.Context())
	if err != nil {
		http.Error(rw, err.Error(), http.StatusServiceUnavailable)
		return
	}
	rw.Header().Set("Content-Type", "text/javascript")
	rw.Header().Set("Cache-Control", "no-store")
	refreshPath := "/bldr-dev/frontend-refresh/" + env.result.GetSession().GetId() + ".mjs"
	switch req.URL.Path {
	case "/bldr-dev/frontend-boot.mjs":
		refresh := strconv.Quote(refreshPath)
		entry := strconv.Quote("/" + strings.TrimPrefix(entrypoint, "/"))
		_, _ = io.WriteString(rw, "import "+refresh+"; window.__bldrFrontendEnabled = true; await import("+entry+");\n")
	case refreshPath:
		_, _ = io.WriteString(rw, env.result.GetRefreshRuntime())
		if env.result.GetRefreshRuntime() == "" {
			return
		}
		_, _ = io.WriteString(rw, "\nif (!globalThis.__bldrReactRefresh) {\n"+
			"globalThis.__bldrReactRefresh = {"+refreshExports+"};\n"+
			"injectIntoGlobalHook(window); window.$RefreshReg$ = () => {}; window.$RefreshSig$ = () => type => type; window.__vite_plugin_react_preamble_installed__ = true;\n}\n")
	default:
		http.Error(rw, "frontend session expired", http.StatusGone)
	}
}

// GetFrontendService returns the development service, if enabled for this project.
func (c *Controller) GetFrontendService() *FrontendService { return c.frontend }

// _ asserts that the project service implements the frontend RPC contract.
var _ frontend.SRPCFrontendServer = (*FrontendService)(nil)
