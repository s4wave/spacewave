package debug_bridge

import (
	"context"

	"github.com/google/uuid"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	web_view "github.com/s4wave/spacewave/bldr/web/view"
	s4wave_debug "github.com/s4wave/spacewave/sdk/debug"
	"github.com/sirupsen/logrus"
)

// bridgeService proxies DebugBridgeService RPCs to the page via WebView.
type bridgeService struct {
	ctrl *Controller
	wv   web_view.WebView
	le   *logrus.Entry
}

// EvalJS evaluates JavaScript code in the page context.
func (s *bridgeService) EvalJS(ctx context.Context, req *s4wave_debug.EvalJSRequest) (*s4wave_debug.EvalJSResponse, error) {
	code := req.GetCode()
	if code == "" {
		return &s4wave_debug.EvalJSResponse{Error: "empty code"}, nil
	}
	truncated := code
	if len(truncated) > 100 {
		truncated = truncated[:100] + "..."
	}
	s.le.Infof("EvalJS: %s", truncated)

	// Store the code as an ES module and get the import URL.
	id := uuid.New().String()
	url := s.ctrl.StoreEvalScript(id, code, req.GetIsModule())
	defer s.ctrl.RemoveEvalScript(id)

	// Proxy to the page with the URL instead of code.
	client := s4wave_debug.NewSRPCDebugBridgeServiceClient(s.wv.GetClient())
	return client.EvalJS(ctx, &s4wave_debug.EvalJSRequest{Url: url})
}

// GetPageInfo returns information about the current page.
func (s *bridgeService) GetPageInfo(ctx context.Context, req *s4wave_debug.GetPageInfoRequest) (*s4wave_debug.GetPageInfoResponse, error) {
	client := s4wave_debug.NewSRPCDebugBridgeServiceClient(s.wv.GetClient())
	resp, err := client.GetPageInfo(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "page info")
	}
	return resp, nil
}

// PluginRpc forwards an RPC stream to a plugin.
// Component ID: plugin id (e.g. "spacewave-core")
func (s *bridgeService) PluginRpc(strm s4wave_debug.SRPCDebugBridgeService_PluginRpcStream) error {
	return rpcstream.HandleRpcStream(
		strm,
		func(ctx context.Context, pluginID string, released func()) (srpc.Invoker, func(), error) {
			if pluginID == "" {
				return nil, nil, errors.New("plugin id required")
			}
			client, clientRef, err := bldr_plugin.ExPluginLoadWaitClient(ctx, s.ctrl.GetBus(), pluginID, nil)
			if err != nil {
				return nil, nil, err
			}
			return srpc.NewClientInvoker(client), clientRef.Release, nil
		},
	)
}

// _ is a type assertion
var _ s4wave_debug.SRPCDebugBridgeServiceServer = ((*bridgeService)(nil))
