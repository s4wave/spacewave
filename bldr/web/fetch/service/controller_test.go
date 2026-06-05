package web_fetch_service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	handlerCtrl := &fetchServiceHTTPHandlerController{
		pathPrefix: "/fs/",
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/fs/proof.txt" {
				t.Fatalf("request path = %q, want /fs/proof.txt", r.URL.Path)
			}
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

	fetchCtrl := NewController(le, b, NewConfig())
	mux := srpc.NewMux()
	if err := web_fetch.SRPCRegisterFetchService(mux, fetchCtrl); err != nil {
		t.Fatal(err)
	}
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
	fetchClient := web_fetch.NewSRPCFetchServiceClient(client)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test/fs/proof.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	rw := httptest.NewRecorder()
	if err := web_fetch.Fetch(ctx, fetchClient.Fetch, req, rw); err != nil {
		t.Fatal(err)
	}

	resp := rw.Result()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain" {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "fetch proof" {
		t.Fatalf("body = %q, want fetch proof", string(body))
	}
}

func TestServeHTTPReturnsNotFoundWhenLookupIsIdle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	ctrl := NewController(le, b, &Config{NotFoundIfIdle: true})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test/missing", nil)
	if err != nil {
		t.Fatal(err)
	}
	rw := httptest.NewRecorder()
	ctrl.ServeHTTP(rw, req)

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
		if c.pathPrefix == "" || len(d.LookupHTTPHandlerURL().Path) >= len(c.pathPrefix) && d.LookupHTTPHandlerURL().Path[:len(c.pathPrefix)] == c.pathPrefix {
			return directive.R(bifrost_http.NewLookupHTTPHandlerResolver(c.handler), nil)
		}
	}
	return nil, nil
}

func (c *fetchServiceHTTPHandlerController) Close() error {
	return nil
}

var _ controller.Controller = ((*fetchServiceHTTPHandlerController)(nil))
