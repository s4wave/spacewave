//go:build !js

package devtool

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
	}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation before listening, got %v", err)
	}
}

func TestListenAndServeDevtoolHTTPServesWhileOnListeningBlocked(t *testing.T) {
	callbackErr := errors.New("startup manifest preflight failed")
	tests := []struct {
		name               string
		callbackErr        error
		cancelWhileBlocked bool
	}{
		{name: "cancellation", cancelWhileBlocked: true},
		{name: "on-listening error", callbackErr: callbackErr},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deadlineCtx, stopDeadline := context.WithTimeout(context.Background(), 10*time.Second)
			t.Cleanup(stopDeadline)
			ctx, cancel := context.WithCancel(deadlineCtx)
			listeningAddr := make(chan string, 1)
			release := make(chan struct{})
			server := &http.Server{
				Addr:              "127.0.0.1:0",
				ReadHeaderTimeout: time.Second,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = io.WriteString(w, "ready")
				}),
			}
			helperDone := make(chan error, 1)
			var releaseOnce sync.Once
			releaseOnListening := func() {
				releaseOnce.Do(func() {
					close(release)
				})
			}
			helperExited := false
			t.Cleanup(func() {
				releaseOnListening()
				cancel()
				_ = server.Close()
				if helperExited {
					return
				}
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cleanupCancel()
				select {
				case <-helperDone:
					helperExited = true
				case <-cleanupCtx.Done():
					t.Errorf("listenAndServeDevtoolHTTP did not exit during cleanup: %v", cleanupCtx.Err())
				}
			})

			go func() {
				helperDone <- listenAndServeDevtoolHTTP(ctx, server, func(addr string) error {
					listeningAddr <- addr
					<-release
					return test.callbackErr
				})
			}()

			var serverAddr string
			select {
			case serverAddr = <-listeningAddr:
			case <-deadlineCtx.Done():
				t.Fatalf("onListening did not start: %v", deadlineCtx.Err())
			}

			client := &http.Client{
				Transport: &http.Transport{DisableKeepAlives: true},
				Timeout:   5 * time.Second,
			}
			response, err := client.Get("http://" + serverAddr)
			if err != nil {
				t.Fatalf("GET while onListening was blocked: %v", err)
			}
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				t.Fatalf("read response while onListening was blocked: %v", err)
			}
			if response.StatusCode != http.StatusOK {
				t.Fatalf("GET status while onListening was blocked: got %d, want %d", response.StatusCode, http.StatusOK)
			}
			if got := string(body); got != "ready" {
				t.Fatalf("GET body while onListening was blocked: got %q, want %q", got, "ready")
			}

			if test.cancelWhileBlocked {
				cancel()
			}
			releaseOnListening()
			select {
			case err := <-helperDone:
				helperExited = true
				if !errors.Is(err, test.callbackErr) {
					t.Fatalf("listenAndServeDevtoolHTTP returned %v, want %v", err, test.callbackErr)
				}
			case <-deadlineCtx.Done():
				t.Fatalf("listenAndServeDevtoolHTTP did not exit: %v", deadlineCtx.Err())
			}

			conn, err := net.DialTimeout("tcp", serverAddr, time.Second)
			if err == nil {
				_ = conn.Close()
				t.Fatal("HTTP listener remained open after listenAndServeDevtoolHTTP returned")
			}
		})
	}
}
