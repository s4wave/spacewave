package bifrost_http

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/testbed"
)

var mockBody = []byte("hello world")

func startMockHandler(t *testing.T, tb *testbed.Testbed) func() {
	// create a http handler
	handler := http.NewServeMux()
	handler.HandleFunc("/bar", func(rw http.ResponseWriter, req *http.Request) {
		tb.Logger.Debugf("got request at url: %s", req.URL.String())
		rw.WriteHeader(200)
		_, _ = rw.Write(mockBody)
	})

	// attach it to the bus
	handlerCtrl := NewHTTPHandlerController(
		controller.NewInfo("bifrost/http/test-handler", semver.MustParse("0.0.1"), "test handler"),
		NewHTTPHandlerBuilder(handler),
		[]string{"/foo"},
		true,
		nil,
	)
	relHandler, err := tb.Bus.AddController(tb.Context, handlerCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return relHandler
}

func checkMockRequest(t *testing.T, handler http.Handler) {
	req := httptest.NewRequest("GET", "/foo/bar", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 status but got %v: %s", resp.StatusCode, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(mockBody, data) {
		t.Fatalf("expected body %v != %v", string(data), string(mockBody))
	}
}
