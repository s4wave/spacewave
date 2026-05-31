//go:build goscript

package goscript_resource_service

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/aperturerobotics/starpc/echo"
	e2e_mock "github.com/aperturerobotics/starpc/mock"
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

	pluginIO, err := web_runtime_wasm.GlobalWasmPluginIo()
	if err != nil {
		postFailure(err)
		return
	}

	viewerRegistry := resource_viewer_registry.NewViewerRegistryResource()
	rootMux := srpc.NewMux(viewerRegistry.GetMux())
	if err := e2e_mock.SRPCRegisterMock(rootMux, nestedResourceMock{}); err != nil {
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

type nestedResourceMock struct{}

func (nestedResourceMock) MockRequest(ctx context.Context, msg *e2e_mock.MockMsg) (*e2e_mock.MockMsg, error) {
	if msg.GetBody() == "create-echo-child" {
		resourceClient, err := resource_server.MustGetResourceClientContext(ctx)
		if err != nil {
			return nil, err
		}
		childMux := srpc.NewMux()
		if err := echo.NewEchoServer(nil).Register(childMux); err != nil {
			return nil, err
		}
		childID, err := resourceClient.AddResource(childMux, nil)
		if err != nil {
			return nil, err
		}
		return &e2e_mock.MockMsg{Body: "echo-child:" + strconv.FormatUint(uint64(childID), 10)}, nil
	}

	engineID, ok := strings.CutPrefix(msg.GetBody(), "run-nested:")
	if !ok {
		return &e2e_mock.MockMsg{Body: "unknown"}, nil
	}

	engineResourceID, err := strconv.ParseUint(engineID, 10, 32)
	if err != nil {
		return nil, err
	}
	resourceClient, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	engine, err := resourceClient.GetAttachedResource(uint32(engineResourceID))
	if err != nil {
		return nil, err
	}
	engineClient := e2e_mock.NewSRPCMockClient(engine)
	childResp, err := engineClient.MockRequest(ctx, &e2e_mock.MockMsg{Body: "create-child"})
	if err != nil {
		return nil, err
	}
	childID, ok := strings.CutPrefix(childResp.GetBody(), "child:")
	if !ok {
		return nil, errors.Errorf("unexpected child response: %q", childResp.GetBody())
	}
	childResourceID, err := strconv.ParseUint(childID, 10, 32)
	if err != nil {
		return nil, err
	}

	child, err := resourceClient.GetAttachedResource(uint32(childResourceID))
	if err != nil {
		return nil, err
	}
	childClient := e2e_mock.NewSRPCMockClient(child)
	childCallResp, err := childClient.MockRequest(ctx, &e2e_mock.MockMsg{Body: "child-check"})
	if err != nil {
		return nil, err
	}
	if childCallResp.GetBody() != "child-ok" {
		return nil, errors.Errorf("unexpected child call response: %q", childCallResp.GetBody())
	}

	releaseResp, err := engineClient.MockRequest(ctx, &e2e_mock.MockMsg{Body: "release-child:" + childID})
	if err != nil {
		return nil, err
	}
	if releaseResp.GetBody() != "released-ok" {
		return nil, errors.Errorf("unexpected child release response: %q", releaseResp.GetBody())
	}
	afterReleaseResp, err := engineClient.MockRequest(ctx, &e2e_mock.MockMsg{Body: "after-release"})
	if err != nil {
		return nil, err
	}
	if afterReleaseResp.GetBody() != "after-release-ok" {
		return nil, errors.Errorf("unexpected after-release response: %q", afterReleaseResp.GetBody())
	}

	return &e2e_mock.MockMsg{Body: "seed-ok"}, nil
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
