package unixfs_access_http

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bifrost_http "github.com/aperturerobotics/bifrost/http"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/testbed"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	unixfs_world_testbed "github.com/aperturerobotics/hydra/unixfs/world/testbed"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	"github.com/blang/semver/v4"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/sirupsen/logrus"
)

func TestHTTPHandlerController(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(true))
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey := "test-fs"
	rootRef, tb, err := unixfs_world_testbed.BuildTestbed(
		btb,
		objKey,
		true,
		world_testbed.WithWorldVerbose(true),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rootRef.Release()

	rbfs := unixfs_billy.NewBillyFS(ctx, rootRef, "", time.Now())
	testData := []byte("hello world")
	if err := billy_util.WriteFile(rbfs, "/bat/baz/test-file.txt", testData, 0o755); err != nil {
		t.Fatal(err.Error())
	}

	testJsData := []byte("console.log(\"hello world\")\n")
	if err := billy_util.WriteFile(rbfs, "/bat/baz/script.js", testJsData, 0o755); err != nil {
		t.Fatal(err.Error())
	}

	// construct the AccessUnixFS handler
	unixFsID := "test-fs"
	accessCtrl := unixfs_access.NewControllerWithHandle(
		tb.Logger,
		tb.Bus,
		controller.NewInfo("hydra/unixfs/access/test", semver.MustParse("0.0.1"), "access test unixfs"),
		[]string{unixFsID},
		rootRef,
	)
	accessRel, err := tb.Bus.AddController(ctx, accessCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer accessRel()

	// construct the http handler
	handlerCtrl := NewController(
		tb.Bus,
		controller.NewInfo("hydra/unixfs/access/test-handler", semver.MustParse("0.0.1"), "test handler"),
		[]string{"/foo/"},
		true,
		nil,
		unixFsID,
		"bat",
		"bar",
		false,
	)
	handlerRel, err := tb.Bus.AddController(ctx, handlerCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer handlerRel()

	// perform a test request via LookupHTTPHandler
	busHandler := bifrost_http.NewBusHandler(tb.Bus, "test-client", false)

	// /bar/ is stripped by the http handler
	// /foo/ in the URL path is stripped by the http handler controller
	// /bat/ is added by the FS prefixer.
	req := httptest.NewRequest("GET", "/foo/bar/baz/test-file.txt", nil)
	rw := httptest.NewRecorder()
	busHandler.ServeHTTP(rw, req)

	res := rw.Result()
	if res.StatusCode != 200 {
		t.Fatalf("status code: %d", res.StatusCode)
	}
	readData, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(readData, testData) {
		t.Fatalf("read data does not match test data: %#v", string(readData))
	}

	// second request: test the mime type of a .js file
	req = httptest.NewRequest("GET", "/foo/bar/baz/script.js", nil)
	rw = httptest.NewRecorder()
	busHandler.ServeHTTP(rw, req)

	res = rw.Result()
	if res.StatusCode != 200 {
		t.Fatalf("status code: %d", res.StatusCode)
	}
	readData, err = io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(readData, testJsData) {
		t.Fatalf("read data does not match test data: %#v", string(readData))
	}
	if contentType := res.Header.Get("content-type"); !strings.HasPrefix(contentType, "text/javascript") {
		t.Fatalf("incorrect content type: %s", contentType)
	}
}
