//go:build !js

package web_pkg_http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	httplog "github.com/aperturerobotics/bifrost/http/log"
	"github.com/aperturerobotics/bldr/core"
	web_pkg_controller "github.com/aperturerobotics/bldr/web/pkg/controller"
	web_pkg_mock "github.com/aperturerobotics/bldr/web/pkg/mock"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

func TestWebPkgHttpServer(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	mockWebPkg := web_pkg_mock.NewMockWebPkg()
	ctrl := web_pkg_controller.NewControllerWithWebPkg(
		le,
		controller.NewInfo("web/pkg/static/test", semver.MustParse("0.0.1"), "test web pkg"),
		mockWebPkg,
	)

	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	rel, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	server := NewServer(le, b, true)
	httpServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		server.ServeWebModuleHTTP(req.URL.Path, rw, req)
	}))
	httpClient := httpServer.Client()

	req, err := http.NewRequest("GET", httpServer.URL+"/"+mockWebPkg.GetId()+"/testdir/testing.txt", nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	resp, err := httplog.DoRequest(le, httpClient, req, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status code 200 got %v", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(data, []byte("file within a directory")) {
		t.Fatalf("got unexpected contents: %v", string(data))
	}
}
