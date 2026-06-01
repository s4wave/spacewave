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
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
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
	if msg.GetBody() == "create-space-child" {
		resourceClient, err := resource_server.MustGetResourceClientContext(ctx)
		if err != nil {
			return nil, err
		}
		childMux := srpc.NewMux()
		if err := s4wave_space.SRPCRegisterSpaceResourceService(childMux, spaceResourceMock{}); err != nil {
			return nil, err
		}
		childID, err := resourceClient.AddResource(childMux, nil)
		if err != nil {
			return nil, err
		}
		return &e2e_mock.MockMsg{Body: "space-child:" + strconv.FormatUint(uint64(childID), 10)}, nil
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

type spaceResourceMock struct{}

func (spaceResourceMock) WatchSpaceState(
	req *s4wave_space.WatchSpaceStateRequest,
	strm s4wave_space.SRPCSpaceResourceService_WatchSpaceStateStream,
) error {
	if err := strm.Send(&s4wave_space.SpaceState{Ready: true}); err != nil {
		return err
	}
	<-strm.Context().Done()
	return nil
}

func (spaceResourceMock) WatchSpaceSharingState(
	req *s4wave_space.WatchSpaceSharingStateRequest,
	strm s4wave_space.SRPCSpaceResourceService_WatchSpaceSharingStateStream,
) error {
	if err := strm.Send(&s4wave_space.SpaceSharingState{}); err != nil {
		return err
	}
	<-strm.Context().Done()
	return nil
}

func (spaceResourceMock) AccessWorld(
	ctx context.Context,
	req *s4wave_space.AccessWorldRequest,
) (*s4wave_space.AccessWorldResponse, error) {
	return nil, errors.New("AccessWorld not implemented")
}

func (spaceResourceMock) MountSpaceContents(
	ctx context.Context,
	req *s4wave_space.MountSpaceContentsRequest,
) (*s4wave_space.MountSpaceContentsResponse, error) {
	return &s4wave_space.MountSpaceContentsResponse{ResourceId: 4242}, nil
}

func (spaceResourceMock) CreateSecret(
	ctx context.Context,
	req *s4wave_space.CreateSecretRequest,
) (*s4wave_space.CreateSecretResponse, error) {
	return nil, errors.New("CreateSecret not implemented")
}

func (spaceResourceMock) ReadSecretPayload(
	ctx context.Context,
	req *s4wave_space.ReadSecretPayloadRequest,
) (*s4wave_space.ReadSecretPayloadResponse, error) {
	return nil, errors.New("ReadSecretPayload not implemented")
}

func (spaceResourceMock) DeployManifest(
	strm s4wave_space.SRPCSpaceResourceService_DeployManifestStream,
) error {
	return errors.New("DeployManifest not implemented")
}

func (spaceResourceMock) AddSpacePlugin(
	ctx context.Context,
	req *s4wave_space.AddSpacePluginRequest,
) (*s4wave_space.AddSpacePluginResponse, error) {
	return nil, errors.New("AddSpacePlugin not implemented")
}

func (spaceResourceMock) RemoveSpacePlugin(
	ctx context.Context,
	req *s4wave_space.RemoveSpacePluginRequest,
) (*s4wave_space.RemoveSpacePluginResponse, error) {
	return nil, errors.New("RemoveSpacePlugin not implemented")
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
