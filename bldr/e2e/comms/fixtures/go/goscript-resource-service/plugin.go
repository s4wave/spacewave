//go:build goscript

package goscript_resource_service

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"syscall/js"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	web_runtime_wasm "github.com/s4wave/spacewave/bldr/web/runtime/wasm"
	resource_viewer_registry "github.com/s4wave/spacewave/core/resource/viewer/registry"
)

func main() {
	go startResourceService()
	select {}
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

	viewerRegistry := resource_viewer_registry.NewViewerRegistryResource()
	rootMux := srpc.NewMux(viewerRegistry.GetMux())
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
