package debug_bridge

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	web_view "github.com/s4wave/spacewave/bldr/web/view"
	debug_projectroot "github.com/s4wave/spacewave/core/debug/projectroot"
	bifrost_http "github.com/s4wave/spacewave/net/http"
	s4wave_debug "github.com/s4wave/spacewave/sdk/debug"
)

// ControllerID is the controller identifier.
const ControllerID = "debug/bridge"

// DebugBridgeWebViewID is the fixed WebView ID for the debug bridge.
const DebugBridgeWebViewID = "debug-bridge"

// defaultSocketPath is the default path for the debug socket.
const defaultSocketPath = ".bldr/alpha-debug.sock"

// Version is the component version.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "debug bridge unix socket rpc controller"

// evalPathPrefix is the URL path prefix for eval scripts.
// Note: the /p/{plugin-id}/ prefix is stripped by the web runtime before reaching ServeHTTP.
const evalPathPrefix = "/eval/"

// Controller is the debug bridge controller.
type Controller struct {
	*bus.BusController[*Config]

	// mtx guards evalScripts.
	mtx sync.Mutex
	// evalScripts stores pending eval scripts keyed by UUID.
	evalScripts map[string]string
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{BusController: base}, nil
		},
	)
}

// getSocketPath returns the absolute socket path.
// Walks up from cwd to find the project root marker.
func (c *Controller) getSocketPath() (string, error) {
	if p := c.GetConfig().GetSocketPath(); p != "" {
		return filepath.Abs(p)
	}
	projectRoot, err := debug_projectroot.FindFromCwd(20)
	if err != nil {
		return "", err
	}
	return filepath.Join(projectRoot, defaultSocketPath), nil
}

func (c *Controller) getEvalOutputDir() (string, error) {
	projectRoot, err := debug_projectroot.FindFromCwd(20)
	if err != nil {
		return "", err
	}
	return filepath.Join(projectRoot, ".bldr", "debug", "eval", "out"), nil
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	le := c.GetLogger()
	b := c.GetBus()

	// Resolve the debug bridge WebView.
	le.Info("waiting for debug bridge web view")
	wv, _, wvRef, err := web_view.ExLookupWebView(
		ctx, b,
		false,
		DebugBridgeWebViewID,
		true, nil,
	)
	if err != nil {
		return err
	}
	defer wvRef.Release()

	// Build mux for the debug bridge service.
	mux := srpc.NewMux()

	// Register the debug bridge service (proxies to page via WebView client).
	svc := &bridgeService{ctrl: c, wv: wv, le: le}
	if err := s4wave_debug.SRPCRegisterDebugBridgeService(mux, svc); err != nil {
		return err
	}

	// Create Unix socket listener.
	// In WASM environments os.Getwd and Unix sockets are unavailable.
	// Return nil so the controller stays attached without retrying.
	absPath, err := c.getSocketPath()
	if err != nil {
		le.WithError(err).Debug("debug bridge socket unavailable, skipping")
		return nil
	}

	// Remove stale socket.
	_ = os.Remove(absPath)

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}

	lis, err := net.Listen("unix", absPath)
	if err != nil {
		return err
	}
	defer func() {
		lis.Close()
		_ = os.Remove(absPath)
	}()

	// Restrict socket permissions to owner only.
	if err := os.Chmod(absPath, 0o600); err != nil {
		le.WithError(err).Warn("failed to chmod socket")
	}

	le.Infof("debug bridge listening on %s", absPath)

	// Close listener when context is cancelled.
	go func() {
		<-ctx.Done()
		lis.Close()
	}()

	srv := srpc.NewServer(mux)
	return srpc.AcceptMuxedListener(ctx, lis, srv, nil)
}

// statementPrefixes lists keywords that indicate the code contains statements
// and should not be implicitly wrapped with return.
var statementPrefixes = []string{
	"var ", "let ", "const ", "if ", "if(", "for ", "for(",
	"while ", "while(", "do ", "do{", "switch ", "switch(",
	"function ", "class ", "try ", "try{", "throw ",
	"return ", "return;", "import ", "export ", "{", "//", "/*",
}

// isExpression returns true if code looks like a single expression
// (no semicolons, single line, no statement keywords).
func isExpression(code string) bool {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" || strings.Contains(trimmed, ";") || strings.Contains(trimmed, "\n") {
		return false
	}
	lower := strings.ToLower(trimmed)
	for _, prefix := range statementPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	return true
}

// StoreEvalScript stores a script and returns the URL path to import it.
// When isModule is true, the code is stored as-is (already a full ES module).
func (c *Controller) StoreEvalScript(id, code string, isModule bool) string {
	stored := code
	if !isModule {
		body := code
		if isExpression(code) {
			body = "return (" + code + ")"
		}
		stored = "export default await (async () => {\n" + body + "\n})()\n"
	}
	c.mtx.Lock()
	if c.evalScripts == nil {
		c.evalScripts = make(map[string]string)
	}
	c.evalScripts[id] = stored
	c.mtx.Unlock()
	// Return the full path the browser uses; the /p/spacewave-debug/ prefix
	// is stripped by the web runtime before reaching our ServeHTTP.
	return "/p/spacewave-debug" + evalPathPrefix + id + ".js"
}

// RemoveEvalScript removes a stored eval script.
func (c *Controller) RemoveEvalScript(id string) {
	c.mtx.Lock()
	delete(c.evalScripts, id)
	c.mtx.Unlock()
}

// ServeHTTP serves eval scripts via HTTP.
func (c *Controller) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	if !strings.HasPrefix(path, evalPathPrefix) {
		http.NotFound(rw, req)
		return
	}
	name := strings.TrimPrefix(path, evalPathPrefix)
	id := strings.TrimSuffix(name, ".js")
	c.mtx.Lock()
	script, ok := c.evalScripts[id]
	c.mtx.Unlock()
	if !ok {
		outDir, err := c.getEvalOutputDir()
		if err != nil {
			http.NotFound(rw, req)
			return
		}
		base := filepath.Base(name)
		if base != name {
			http.NotFound(rw, req)
			return
		}
		http.ServeFile(rw, req, filepath.Join(outDir, base))
		return
	}
	rw.Header().Set("Content-Type", "application/javascript")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write([]byte(script))
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch di.GetDirective().(type) {
	case bifrost_http.LookupHTTPHandler:
		return directive.R(bifrost_http.NewLookupHTTPHandlerResolver(c), nil)
	}
	return nil, nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
