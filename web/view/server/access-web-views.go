package web_view_server

import (
	"context"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	web_view "github.com/aperturerobotics/bldr/web/view"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// AccessWebViewsViaBusControllerID is the controller ID used for AccessWebViewsViaBus.
const AccessWebViewsViaBusControllerID = "bldr/web/view/access/via-bus"

// AccessWebViewsViaBusVersion is the controller version used for AccessWebViewsViaBus.
var AccessWebViewsViaBusVersion = semver.MustParse("0.0.1")

// AccessWebViewsViaBus implements the AccessWebViews service.
type AccessWebViewsViaBus struct {
	le *logrus.Entry
	b  bus.Bus
}

// NewAccessWebViewsViaBus constructs a AccessWebViewsViaBus service.
func NewAccessWebViewsViaBus(
	le *logrus.Entry,
	b bus.Bus,
) *AccessWebViewsViaBus {
	return &AccessWebViewsViaBus{
		le: le,
		b:  b,
	}
}

// NewAccessWebViewsViaBusController constructs a new controller resolving
// LookupRpcService with the AccessWebViewsViaBus service.
func NewAccessWebViewsViaBusController(
	le *logrus.Entry,
	b bus.Bus,
) *bifrost_rpc.InvokerController {
	mux := srpc.NewMux()
	f := NewAccessWebViewsViaBus(le, b)
	_ = web_view.SRPCRegisterAccessWebViews(mux, f)
	return bifrost_rpc.NewInvokerController(
		le,
		b,
		controller.NewInfo(
			AccessWebViewsViaBusControllerID,
			AccessWebViewsViaBusVersion,
			"AccessWebViews rpc to directive",
		),
		mux,
		nil,
	)
}

// GetWebViewInvoker returns the Invoker for the given web view id.
func (h *AccessWebViewsViaBus) GetWebViewInvoker(
	ctx context.Context,
	webViewID string,
	released func(),
) (srpc.Invoker, func(), error) {
	webView, _, webViewRef, err := web_view.ExLookupWebView(ctx, h.b, false, webViewID, true, released)
	if err != nil {
		return nil, nil, err
	}
	handler := web_view.NewSRPCWebViewHandler(NewWebViewServer(webView), "")
	mux := srpc.NewMux(srpc.NewClientInvoker(webView.GetClient()))
	if err := mux.Register(handler); err != nil {
		webViewRef.Release()
		return nil, nil, err
	}
	return mux, webViewRef.Release, nil
}

// WebViewRpc accesses the WebView service for a view by ID.
// Id: web view id
func (h *AccessWebViewsViaBus) WebViewRpc(strm web_view.SRPCAccessWebViews_WebViewRpcStream) error {
	return rpcstream.HandleRpcStream(strm, h.GetWebViewInvoker)
}

// _ is a type assertion
var _ web_view.SRPCAccessWebViewsServer = ((*AccessWebViewsViaBus)(nil))
