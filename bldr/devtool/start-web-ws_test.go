//go:build !js

package devtool

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/go-websocket"
	entrypoint_browser_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/bundle"
)

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

// TestListenAndServeDevtoolWebWsClosesUpgradedControllers drives the production
// websocket serve path: an upgraded connection whose controller only exits when
// the socket closes. The server must close every upgraded connection and wait
// for its controller before listenAndServeDevtoolWebWs returns.
func TestListenAndServeDevtoolWebWsClosesUpgradedControllers(t *testing.T) {
	deadlineCtx, stopDeadline := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopDeadline()
	ctx, cancel := context.WithCancel(deadlineCtx)
	connections := newWebSocketControllerConnections(ctx)
	handlerStarted := make(chan struct{})
	handlerExited := make(chan struct{})
	server := &http.Server{
		Addr:              "127.0.0.1:0",
		ReadHeaderTimeout: time.Second,
		Handler: http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			connection, err := websocket.Accept(rw, req, nil)
			if err != nil {
				return
			}
			_ = connections.Execute(connection, func(context.Context) error {
				close(handlerStarted)
				// Read until the socket closes; do not return on cancellation.
				readCtx := context.WithoutCancel(deadlineCtx)
				_, _, readErr := connection.Read(readCtx)
				close(handlerExited)
				return readErr
			})
		}),
	}
	listeningAddr := make(chan string, 1)
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- listenAndServeDevtoolWebWs(ctx, server, connections, func(addr string) error {
			listeningAddr <- addr
			return nil
		})
	}()

	addr := <-listeningAddr
	client, _, err := websocket.Dial(deadlineCtx, "ws://"+addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseNow()
	<-handlerStarted
	cancel()

	select {
	case err := <-serverDone:
		select {
		case <-handlerExited:
		default:
			t.Fatal("server returned before the upgraded WebSocket controller exited")
		}
		if err != nil {
			t.Fatalf("server shutdown: %v", err)
		}
	case <-deadlineCtx.Done():
		t.Fatalf("server did not close and join upgraded controllers: %v", deadlineCtx.Err())
	}

	readCtx, stopRead := context.WithTimeout(context.Background(), time.Second)
	defer stopRead()
	if _, _, err := client.Read(readCtx); err == nil {
		t.Fatal("WebSocket remained open after server shutdown")
	}
}

// TestListenAndServeDevtoolHTTPDrainsPlainRequestsOnCancel pins the graceful
// plain HTTP path: an in-flight request finishes and the caller sees a normal
// response even though cancellation started while the request was active.
func TestListenAndServeDevtoolHTTPDrainsPlainRequestsOnCancel(t *testing.T) {
	deadlineCtx, stopDeadline := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopDeadline()
	ctx, cancel := context.WithCancel(deadlineCtx)
	requestStarted := make(chan struct{})
	server := &http.Server{
		Addr:              "127.0.0.1:0",
		ReadHeaderTimeout: time.Second,
		Handler: http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			close(requestStarted)
			select {
			case <-req.Context().Done():
				return
			case <-time.After(300 * time.Millisecond):
			}
			_, _ = io.WriteString(rw, "drained")
		}),
	}
	listeningAddr := make(chan string, 1)
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- listenAndServeDevtoolHTTP(ctx, server, func(addr string) error {
			listeningAddr <- addr
			return nil
		})
	}()

	addr := <-listeningAddr
	type httpResult struct {
		body string
		err  error
	}
	resultCh := make(chan httpResult, 1)
	go func() {
		resp, err := http.Get("http://" + addr)
		if err != nil {
			resultCh <- httpResult{err: err}
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		resultCh <- httpResult{body: string(body), err: err}
	}()
	<-requestStarted
	cancel()

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("in-flight request was not drained: %v", result.err)
		}
		if result.body != "drained" {
			t.Fatalf("drained request body: got %q, want %q", result.body, "drained")
		}
	case <-deadlineCtx.Done():
		t.Fatalf("in-flight request did not complete during shutdown: %v", deadlineCtx.Err())
	}

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("server shutdown: %v", err)
		}
	case <-deadlineCtx.Done():
		t.Fatalf("server did not finish draining: %v", deadlineCtx.Err())
	}
}

// TestWebSocketControllerConnectionsRejectExecuteAfterClose pins post-close
// rejection: Execute after Close force-closes the connection, reports an
// error, and never runs the controller.
func TestWebSocketControllerConnectionsRejectExecuteAfterClose(t *testing.T) {
	deadlineCtx, stopDeadline := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopDeadline()
	ctx, cancel := context.WithCancel(deadlineCtx)
	defer cancel()
	connections := newWebSocketControllerConnections(ctx)
	accepted := make(chan struct{})
	closeGate := make(chan struct{})
	executeErr := make(chan error, 1)
	executed := make(chan struct{})
	server := &http.Server{
		Addr:              "127.0.0.1:0",
		ReadHeaderTimeout: time.Second,
		Handler: http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			connection, err := websocket.Accept(rw, req, nil)
			if err != nil {
				return
			}
			close(accepted)
			<-closeGate
			executeErr <- connections.Execute(connection, func(context.Context) error {
				close(executed)
				return nil
			})
		}),
	}
	listeningAddr := make(chan string, 1)
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- listenAndServeDevtoolWebWs(ctx, server, connections, func(addr string) error {
			listeningAddr <- addr
			return nil
		})
	}()

	addr := <-listeningAddr
	client, _, err := websocket.Dial(deadlineCtx, "ws://"+addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseNow()
	<-accepted

	connections.Close()
	close(closeGate)

	select {
	case err := <-executeErr:
		if !errors.Is(err, errWebSocketConnectionsClosed) {
			t.Fatalf("Execute after Close: got %v, want %v", err, errWebSocketConnectionsClosed)
		}
	case <-deadlineCtx.Done():
		t.Fatal("Execute after Close did not return")
	}
	select {
	case <-executed:
		t.Fatal("controller ran after Close rejected it")
	default:
	}

	readCtx, stopRead := context.WithTimeout(context.Background(), time.Second)
	defer stopRead()
	if _, _, err := client.Read(readCtx); err == nil {
		t.Fatal("WebSocket remained open after Close rejected it")
	}

	cancel()
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("server shutdown: %v", err)
		}
	case <-deadlineCtx.Done():
		t.Fatalf("server did not shut down: %v", deadlineCtx.Err())
	}
}
