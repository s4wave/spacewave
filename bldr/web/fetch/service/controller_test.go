package web_fetch_service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/core"
	web_fetch "github.com/s4wave/spacewave/bldr/web/fetch"
	bifrost_http "github.com/s4wave/spacewave/net/http"
	"github.com/sirupsen/logrus"
)

func TestFetchResolvesHTTPHandlerThroughBus(t *testing.T) {
	// Bound the HTTP lookup test and release its context afterward.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the controller bus that resolves HTTP handler directives.
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	// Register an HTTP handler that serves the proof path.
	handlerCtrl := &fetchServiceHTTPHandlerController{
		pathPrefix: "/fs/",
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify that the HTTP lookup preserves the requested proof path.
			if r.URL.Path != "/fs/proof.txt" {
				t.Fatalf("request path = %q, want /fs/proof.txt", r.URL.Path)
			}

			// Return the proof response with its content type and accepted status.
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("fetch proof"))
		}),
	}
	handlerRel, err := b.AddController(ctx, handlerCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer handlerRel()

	// Connect the fetch service client to the controller through an RPC pipe.
	fetchCtrl := NewController(le, b, NewConfig())
	mux := srpc.NewMux()
	if err := web_fetch.SRPCRegisterFetchService(mux, fetchCtrl); err != nil {
		t.Fatal(err)
	}
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
	fetchClient := web_fetch.NewSRPCFetchServiceClient(client)

	// Send the proof request and record the HTTP response.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test/fs/proof.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	rw := httptest.NewRecorder()
	if err := web_fetch.Fetch(ctx, fetchClient.Fetch, req, rw); err != nil {
		t.Fatal(err)
	}

	// Verify the proof response status and content type.
	resp := rw.Result()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain" {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}

	// Verify that the HTTP response contains the handler's proof body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "fetch proof" {
		t.Fatalf("body = %q, want fetch proof", string(body))
	}
}

func TestDirectLookupHTTPHandlerThroughBus(t *testing.T) {
	// Bound the HTTP lookup test and release its context afterward.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the controller bus that resolves HTTP handler directives.
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	// Register an HTTP handler that serves the proof path.
	handlerCtrl := &fetchServiceHTTPHandlerController{
		pathPrefix: "/fs/",
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify that the HTTP lookup preserves the requested proof path.
			if r.URL.Path != "/fs/proof.txt" {
				t.Fatalf("request path = %q, want /fs/proof.txt", r.URL.Path)
			}

			// Return the proof response with its content type and accepted status.
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("direct proof"))
		}),
	}
	handlerRel, err := b.AddController(ctx, handlerCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer handlerRel()

	// Resolve the proof URL through the controller bus and retain the handler.
	handlerURL, err := url.Parse("https://example.test/fs/proof.txt")
	if err != nil {
		t.Fatal(err)
	}
	handler, _, handlerRef, err := bifrost_http.ExLookupFirstHTTPHandler(
		ctx,
		b,
		http.MethodGet,
		handlerURL,
		"",
		true,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if handlerRef == nil || handler == nil {
		t.Fatal("handler lookup returned no handler")
	}
	defer handlerRef.Release()

	// Send the proof request and record the HTTP response.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, handlerURL.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	// Verify the proof response status and content type.
	resp := rw.Result()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain" {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}

	// Verify that the HTTP response contains the handler's proof body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "direct proof" {
		t.Fatalf("body = %q, want direct proof", string(body))
	}
}

func TestServeHTTPReturnsNotFoundWhenLookupIsIdle(t *testing.T) {
	// Bound the HTTP lookup test and release its context afterward.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start the controller bus that resolves HTTP handler directives.
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	// Request a missing path from a controller whose idle lookup returns 404.
	ctrl := NewController(le, b, &Config{NotFoundIfIdle: true})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test/missing", nil)
	if err != nil {
		t.Fatal(err)
	}
	rw := httptest.NewRecorder()

	// Run the HTTP request with a completion signal for the idle lookup.
	done := make(chan struct{})
	go func() {
		ctrl.ServeHTTP(rw, req)
		close(done)
	}()

	// Require the idle lookup to finish before the request context expires.
	select {
	case <-done:
	case <-ctx.Done():
		select {
		case <-done:
			t.Fatal("ServeHTTP waited for request context cancellation; want idle lookup to return 404 before the guard context expires")
		case <-time.After(time.Second):
			t.Fatal("ServeHTTP did not return after the guard context expired")
		}
	}

	// Verify that the idle lookup returns the missing-path status.
	if rw.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rw.Code, http.StatusNotFound)
	}
}

type fetchServiceHTTPHandlerController struct {
	pathPrefix string
	handler    http.Handler
}

func (c *fetchServiceHTTPHandlerController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"bldr/web/fetch/service/test-handler",
		controller.MustParseVersion("0.0.1"),
		"test http handler",
	)
}

func (c *fetchServiceHTTPHandlerController) Execute(ctx context.Context) error {
	return nil
}

func (c *fetchServiceHTTPHandlerController) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case bifrost_http.LookupHTTPHandler:
		if c.pathPrefix == "" || strings.HasPrefix(d.LookupHTTPHandlerURL().Path, c.pathPrefix) {
			return directive.R(bifrost_http.NewLookupHTTPHandlerResolver(c.handler), nil)
		}
	}
	return nil, nil
}

func (c *fetchServiceHTTPHandlerController) Close() error {
	return nil
}

var _ controller.Controller = (*fetchServiceHTTPHandlerController)(nil)
