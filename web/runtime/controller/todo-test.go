package web_runtime_controller

// Various hardcoded demos to be removed later.

import (
	"context"
	"net/http"
	"time"

	demo "github.com/aperturerobotics/bldr/toys/test-component"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	billy_util "github.com/go-git/go-billy/v5/util"
	"github.com/sirupsen/logrus"

	// _ embeds data
	_ "embed"
)

// buildExampleFS builds a test unixfs for testing the service worker.
func buildExampleFS(ctx context.Context, le *logrus.Entry) (ufs *unixfs.FS, utb *world_testbed.Testbed, err error) {
	objKey := "example/test/1"
	ufs, utb, err = unixfs_world.BuildTestbed(ctx, objKey, true)
	if err != nil {
		return nil, nil, err
	}

	handle, err := ufs.AddRootReference(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer handle.Release()

	// create test fs (backed by a block graph + Hydra world)
	bfs := unixfs.NewBillyFilesystem(ctx, handle, "", time.Now())

	// create test image
	err = billy_util.WriteFile(bfs, "test.png", demoPng, 0755)
	if err != nil {
		return nil, nil, err
	}

	// create test script
	err = billy_util.WriteFile(bfs, "test.js", []byte(getTestComponentJS()+"\n"), 0755)
	if err != nil {
		return nil, nil, err
	}

	// done
	return ufs, utb, nil
}

func getTestComponentJS() string {
	return demo.TestComponentJS
}

//go:embed test.png
var demoPng []byte

func getTestSwMux(le *logrus.Entry) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/b/test.png", http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		le.Debugf("service worker fetch test png: %s", req.URL.String())
		// TODO: Demo image
		rw.Header().Set("Content-Type", "image/png")
		rw.WriteHeader(200)
		// basic test image
		rw.Write(demoPng)
	}))

	// TODO DEMO
	mux.Handle("/b/test.jsb", http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		le.Debugf("service worker fetch test component: %s", req.URL.String())
		rw.Header().Set("Content-Type", "text/javascript")
		rw.WriteHeader(200)
		rw.Write([]byte(getTestComponentJS() + "\n"))
	}))
	return mux
}
