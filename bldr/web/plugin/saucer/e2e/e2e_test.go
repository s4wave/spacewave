//go:build !js

package saucer_e2e_test

import (
	"context"
	"io"
	"io/fs"
	"net"
	"testing"
	"testing/fstest"

	bldr_core "github.com/s4wave/spacewave/bldr/core"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	web_pkg_controller "github.com/s4wave/spacewave/bldr/web/pkg/controller"
	web_pkg_external "github.com/s4wave/spacewave/bldr/web/pkg/external"
	web_pkg_static "github.com/s4wave/spacewave/bldr/web/pkg/static"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	runtime_controller "github.com/s4wave/spacewave/bldr/web/runtime/controller"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// runtimeID matches the saucer controller's RuntimeID.
const runtimeID = "saucer"

// docID is the simulated browser's document ID.
const docID = "test-document"

// simulatedBrowser implements SRPCWebRuntimeServer.
// It acts as the browser/C++ side of the saucer connection.
type simulatedBrowser struct {
	le *logrus.Entry

	// docMux serves RPC calls for the document.
	// Go opens WebDocumentRpc streams to talk to JS via this mux.
	docMux srpc.Mux
}

// newSimulatedBrowser creates a simulated browser.
func newSimulatedBrowser(le *logrus.Entry) *simulatedBrowser {
	return &simulatedBrowser{
		le:     le,
		docMux: srpc.NewMux(),
	}
}

// WatchWebRuntimeStatus sends an initial snapshot with one document, then blocks.
func (b *simulatedBrowser) WatchWebRuntimeStatus(
	_ *web_runtime.WatchWebRuntimeStatusRequest,
	strm web_runtime.SRPCWebRuntime_WatchWebRuntimeStatusStream,
) error {
	b.le.Debug("WatchWebRuntimeStatus: sending snapshot")
	status := &web_runtime.WebRuntimeStatus{
		Snapshot: true,
		WebDocuments: []*web_runtime.WebDocumentStatus{{
			Id:        docID,
			Permanent: true,
		}},
	}
	if err := strm.Send(status); err != nil {
		return err
	}
	<-strm.Context().Done()
	return nil
}

// CreateWebDocument returns false (saucer doesn't support creating documents).
func (b *simulatedBrowser) CreateWebDocument(
	_ context.Context,
	_ *web_runtime.CreateWebDocumentRequest,
) (*web_runtime.CreateWebDocumentResponse, error) {
	return &web_runtime.CreateWebDocumentResponse{Created: false}, nil
}

// RemoveWebDocument returns false (saucer doesn't support removing documents).
func (b *simulatedBrowser) RemoveWebDocument(
	_ context.Context,
	_ *web_runtime.RemoveWebDocumentRequest,
) (*web_runtime.RemoveWebDocumentResponse, error) {
	return &web_runtime.RemoveWebDocumentResponse{Removed: false}, nil
}

// WebDocumentRpc handles incoming RpcStream calls to the document.
func (b *simulatedBrowser) WebDocumentRpc(strm web_runtime.SRPCWebRuntime_WebDocumentRpcStream) error {
	b.le.Debug("WebDocumentRpc: stream opened")
	return rpcstream.HandleRpcStream(strm, b.getDocumentHost)
}

// getDocumentHost returns the mux for the given document.
func (b *simulatedBrowser) getDocumentHost(ctx context.Context, componentID string, released func()) (srpc.Invoker, func(), error) {
	b.le.WithField("component-id", componentID).Debug("getDocumentHost called")
	return b.docMux, nil, nil
}

// WebWorkerRpc is not used in saucer.
func (b *simulatedBrowser) WebWorkerRpc(_ web_runtime.SRPCWebRuntime_WebWorkerRpcStream) error {
	return nil
}

// _ is a type assertion
var _ web_runtime.SRPCWebRuntimeServer = (*simulatedBrowser)(nil)

// newMockWebPkgFS creates a minimal mock filesystem for a web package.
func newMockWebPkgFS() fs.FS {
	return fstest.MapFS{
		"index.mjs": &fstest.MapFile{Data: []byte("export default {};\n")},
	}
}

// newMockWebPkg creates a mock web package with the given ID.
func newMockWebPkg(id string) web_pkg.WebPkg {
	mockFS := newMockWebPkgFS()
	pkg, _ := web_pkg_static.NewStaticWebPkg(
		&web_pkg.WebPkgInfo{Id: id},
		func(ctx context.Context) (*unixfs.FSHandle, error) {
			fsc, err := unixfs_iofs.NewFSCursor(mockFS)
			if err != nil {
				return nil, err
			}
			return unixfs.NewFSHandle(fsc)
		},
	)
	return pkg
}

// TestSaucerInProcess runs the saucer runtime stack in-process with a simulated browser.
//
// This replicates the exact Go-side flow from saucer.Controller.Execute:
//   - Creates two net.Pipe pairs (main RPC + fetch)
//   - Wraps them with yamux muxed connections
//   - The "browser" side runs SRPCWebRuntimeServer (simulated)
//   - The "Go" side runs runtime_controller.Controller + web_runtime.Remote
//   - Mock web packages are added to the bus
//
// Run:
//
//	go test -v -run TestSaucerInProcess -timeout 5m ./web/plugin/saucer/e2e/
func TestSaucerInProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Create the controller bus.
	b, _, err := bldr_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatalf("create bus: %v", err)
	}

	// Add mock web packages for BldrExternal.
	pkgMap := make(map[string]web_pkg.WebPkg, len(web_pkg_external.BldrExternal))
	for _, pkgID := range web_pkg_external.BldrExternal {
		pkgMap[pkgID] = newMockWebPkg(pkgID)
	}
	pkgCtrl := web_pkg_controller.NewControllerWithWebPkgMap(
		le,
		controller.NewInfo("e2e/web-pkgs", semver.MustParse("0.0.1"), "e2e mock web packages"),
		pkgMap,
	)
	if err := b.ExecuteController(ctx, pkgCtrl); err != nil {
		t.Fatalf("execute web pkg controller: %v", err)
	}

	// Create two net.Pipe pairs: main RPC and fetch.
	mainGoConn, mainBrowserConn := net.Pipe()
	fetchGoConn, fetchBrowserConn := net.Pipe()
	_ = fetchBrowserConn // browser doesn't initiate fetch in this test

	// Create yamux muxed connections.
	// Go side: outbound=false (server, even stream IDs)
	// Browser side: outbound=true (client, odd stream IDs)
	mainGoMc, err := srpc.NewMuxedConn(mainGoConn, false, nil)
	if err != nil {
		t.Fatalf("create main go muxed conn: %v", err)
	}
	mainBrowserMc, err := srpc.NewMuxedConn(mainBrowserConn, true, nil)
	if err != nil {
		t.Fatalf("create main browser muxed conn: %v", err)
	}
	fetchGoMc, err := srpc.NewMuxedConn(fetchGoConn, false, nil)
	if err != nil {
		t.Fatalf("create fetch go muxed conn: %v", err)
	}

	// Set up the simulated browser.
	browser := newSimulatedBrowser(le.WithField("side", "browser"))

	// Register WebRuntime service on the browser's mux and start accepting.
	browserMux := srpc.NewMux()
	if err := web_runtime.SRPCRegisterWebRuntime(browserMux, browser); err != nil {
		t.Fatalf("register browser WebRuntime: %v", err)
	}
	browserServer := srpc.NewServer(browserMux)

	eg, egCtx := errgroup.WithContext(ctx)

	// Browser: accept streams from Go on the main connection.
	eg.Go(func() error {
		err := browserServer.AcceptMuxedConn(egCtx, mainBrowserMc)
		if err == io.EOF || err == context.Canceled {
			return nil
		}
		return err
	})

	// Create the runtime controller (replicates saucer.Controller.Execute).
	rc := runtime_controller.NewController(
		le.WithField("side", "go"),
		b,
		func(
			ctx context.Context,
			le *logrus.Entry,
			handler web_runtime.WebRuntimeHandler,
		) (web_runtime.WebRuntime, error) {
			srpcClient := srpc.NewClientWithMuxedConn(mainGoMc)
			return web_runtime.NewRemote(
				le, b, handler, runtimeID, srpcClient,
				func(ctx context.Context, rem *web_runtime.Remote) error {
					remEg, remCtx := errgroup.WithContext(ctx)

					// Main RPC: accept incoming streams from browser.
					remEg.Go(func() error {
						err := rem.GetRpcServer().AcceptMuxedConn(remCtx, mainGoMc)
						if err == io.EOF || err == context.Canceled {
							return nil
						}
						return err
					})

					// Fetch: accept ServiceWorker RPC streams.
					remEg.Go(func() error {
						err := rem.AcceptServiceWorkerRpcStreams(remCtx, fetchGoMc)
						if err == io.EOF || err == context.Canceled {
							return nil
						}
						return err
					})

					return remEg.Wait()
				},
			)
		},
		"e2e/saucer",
		semver.MustParse("0.0.1"),
	)

	// Execute the runtime controller on the bus.
	eg.Go(func() error {
		err := b.ExecuteController(egCtx, rc)
		if err != nil && err != context.Canceled {
			t.Logf("runtime controller exited: %v", err)
		}
		return err
	})

	// Wait for the runtime to be ready.
	t.Log("waiting for runtime to be ready...")
	rt, err := rc.GetWebRuntime(ctx)
	if err != nil {
		t.Fatalf("get web runtime: %v", err)
	}

	t.Log("waiting for web document...")
	doc, err := rt.GetWebDocument(ctx, docID, true)
	if err != nil {
		t.Fatalf("get web document: %v", err)
	}
	t.Logf("web document ready: id=%s", doc.GetWebDocumentUuid())

	t.Log("all checks passed, shutting down")
	cancel()

	// Close underlying pipes to unblock yamux AcceptStream calls.
	mainGoConn.Close()
	mainBrowserConn.Close()
	fetchGoConn.Close()
	fetchBrowserConn.Close()

	_ = eg.Wait()
}
