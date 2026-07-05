//go:build !js

package devtool

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/fastjson"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
)

func TestExecuteWebWsProjectStartsProjectStartupAfterNativePluginHost(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "start-web-ws.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		declFn, ok := decl.(*ast.FuncDecl)
		if ok && declFn.Name.Name == "ExecuteWebWsProject" {
			fn = declFn
			break
		}
	}
	if fn == nil {
		t.Fatal("ExecuteWebWsProject not found")
	}

	order := make(map[string]int)
	callIndex := 0
	var projectStartupDisabled bool
	var autoStartupProjectController bool
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		callIndex++
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "StartProjectController":
			autoStartupProjectController = true
		case "StartProjectControllerWithStartup":
			order["project-controller"] = callIndex
			lastArg := call.Args[len(call.Args)-1]
			lastIdent, ok := lastArg.(*ast.Ident)
			projectStartupDisabled = ok && lastIdent.Name == "false"
		case "StartPluginScheduler":
			order["plugin-scheduler"] = callIndex
		case "StartPluginHost":
			order["plugin-host"] = callIndex
		case "StartStartup":
			order["project-startup"] = callIndex
		case "setCommandRunningWithLogFile":
			order["runtime-active"] = callIndex
		}
		return true
	})

	if autoStartupProjectController {
		t.Fatal("ExecuteWebWsProject must not use StartProjectController because it auto-starts project startup before the scheduler exists")
	}
	if !projectStartupDisabled {
		t.Fatal("ExecuteWebWsProject must call StartProjectControllerWithStartup with start=false")
	}
	wantOrder := []string{
		"project-controller",
		"plugin-scheduler",
		"plugin-host",
		"project-startup",
		"runtime-active",
	}
	for _, name := range wantOrder {
		if order[name] == 0 {
			t.Fatalf("%s call not found in ExecuteWebWsProject", name)
		}
	}
	for i := 1; i < len(wantOrder); i++ {
		prev, curr := wantOrder[i-1], wantOrder[i]
		if order[prev] >= order[curr] {
			t.Fatalf("%s must run before %s: order=%v", prev, curr, order)
		}
	}
}

func TestWriteWebWsBuildManifest(t *testing.T) {
	dir := t.TempDir()
	err := writeWebWsBuildManifest(dir, &entrypoint_browser_bundle.BrowserBundleResult{
		EntrypointPath:             "entrypoint/entrypoint.mjs",
		EntrypointDecompressedSize: 1234,
		ServiceWorkerFilename:      "sw.mjs",
		SharedWorkerFilename:       "shw.mjs",
		CSSPaths:                   []string{"entrypoint/app.css"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "browser-release.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parser fastjson.Parser
	v, err := parser.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(v.GetStringBytes("shellAssets", "entrypoint")); got != "entrypoint/entrypoint.mjs" {
		t.Fatalf("unexpected entrypoint: %q", got)
	}
	if got := v.GetInt("shellAssets", "entrypointDecompressedSize"); got != 1234 {
		t.Fatalf("unexpected entrypointDecompressedSize: %d", got)
	}
	if got := string(v.GetStringBytes("shellAssets", "wasm")); got != "entrypoint/runtime-ws.mjs" {
		t.Fatalf("unexpected websocket runtime asset: %q", got)
	}

	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
}

func TestListenAndServeDevtoolHTTPDoesNotListenAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := listenAndServeDevtoolHTTP(ctx, &http.Server{
		Addr:              "127.0.0.1:0",
		Handler:           http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		ReadHeaderTimeout: 0,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation before listening, got %v", err)
	}
}
