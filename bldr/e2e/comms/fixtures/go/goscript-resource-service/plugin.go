//go:build goscript

package goscript_resource_service

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"syscall/js"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	web_runtime_wasm "github.com/s4wave/spacewave/bldr/web/runtime/wasm"
)

func main() {
	go startResourceService()
	select {}
}

// registrations is the root resource: Echo registers a child resource and
// EchoServerStream watches how many registrations clients still hold.
type registrations struct {
	*echo.EchoServer

	// bcast guards live and is broadcast when it changes.
	bcast broadcast.Broadcast
	// live is the number of registered child resources not yet released.
	live int
}

// Echo registers one child resource and returns its resource id as the body.
func (r *registrations) Echo(ctx context.Context, _ *echo.EchoMsg) (*echo.EchoMsg, error) {
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	r.addLive(1)
	resourceID, err := client.AddResource(srpc.NewMux(), func() {
		r.addLive(-1)
	})
	if err != nil {
		r.addLive(-1)
		return nil, err
	}
	return &echo.EchoMsg{Body: strconv.FormatUint(uint64(resourceID), 10)}, nil
}

// EchoServerStream sends the live registration count now and on every change.
func (r *registrations) EchoServerStream(
	_ *echo.EchoMsg,
	strm echo.SRPCEchoer_EchoServerStreamStream,
) error {
	ctx := strm.Context()
	for {
		var live int
		var waitCh <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			live = r.live
			waitCh = getWaitCh()
		})

		if err := strm.Send(&echo.EchoMsg{Body: strconv.Itoa(live)}); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// addLive adjusts the live registration count and wakes watchers.
func (r *registrations) addLive(delta int) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.live += delta
		broadcast()
	})
}

func startResourceService() {
	defer func() {
		if recovered := recover(); recovered != nil {
			postFailure(errors.Errorf("panic: %v", recovered))
		}
	}()

	ctx := context.Background()
	startInfo := js.Global().Get("BLDR_PLUGIN_START_INFO")
	postMessage(map[string]any{
		"type":             "start-info",
		"startInfoPresent": startInfo.Truthy(),
		"compiler":         runtime.Compiler,
	})
	fmt.Fprintln(os.Stderr, "goscript resource-service stderr fallback proof")

	pluginIO, err := web_runtime_wasm.GlobalWasmPluginIo()
	if err != nil {
		postFailure(err)
		return
	}

	rootMux := srpc.NewMux()
	root := &registrations{EchoServer: echo.NewEchoServer(nil)}
	if err := echo.SRPCRegisterEchoer(rootMux, root); err != nil {
		postFailure(err)
		return
	}
	resourceServer := resource_server.NewResourceServer(rootMux)
	mux := srpc.NewMux()
	if err := resourceServer.Register(mux); err != nil {
		postFailure(err)
		return
	}

	pluginIO.SetAcceptStreams(ctx, mux)
	postMessage(map[string]any{
		"type":  "accept-ready",
		"ready": true,
	})
}

func postFailure(err error) {
	postMessage(map[string]any{
		"type":          "resource-service-failed",
		"failureReason": err.Error(),
	})
}

func postMessage(msg map[string]any) {
	js.Global().Call("postMessage", js.ValueOf(msg))
}
