package web_view_handler_server

import (
	"context"

	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	web_view "github.com/s4wave/spacewave/bldr/web/view"
	web_view_client "github.com/s4wave/spacewave/bldr/web/view/client"
	web_view_handler "github.com/s4wave/spacewave/bldr/web/view/handler"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// HandleWebViewViaBusControllerID is the controller ID used for HandleWebViewViaBus.
const HandleWebViewViaBusControllerID = "bldr/web/view/handler/via-bus"

// HandleWebViewViaBusVersion is the controller version used for HandleWebViewViaBus.
var HandleWebViewViaBusVersion = semver.MustParse("0.0.1")

// HandleWebViewViaBus implements the HandleWebView service.
type HandleWebViewViaBus struct {
	le           *logrus.Entry
	b            bus.Bus
	accessClient web_view.SRPCAccessWebViewsClient
}

// NewHandleWebViewViaBus constructs a HandleWebViewViaBus service.
func NewHandleWebViewViaBus(
	le *logrus.Entry,
	b bus.Bus,
	accessClient web_view.SRPCAccessWebViewsClient,
) *HandleWebViewViaBus {
	return &HandleWebViewViaBus{
		le:           le,
		b:            b,
		accessClient: accessClient,
	}
}

// NewHandleWebViewViaBusController constructs a new controller resolving
// LookupRpcService with the HandleWebViewViaBus service.
func NewHandleWebViewViaBusController(
	le *logrus.Entry,
	b bus.Bus,
	accessClient web_view.SRPCAccessWebViewsClient,
) *bifrost_rpc.InvokerController {
	mux := srpc.NewMux()
	f := NewHandleWebViewViaBus(le, b, accessClient)
	_ = web_view_handler.SRPCRegisterHandleWebViewService(mux, f)
	return bifrost_rpc.NewInvokerController(
		le,
		b,
		controller.NewInfo(
			HandleWebViewViaBusControllerID,
			HandleWebViewViaBusVersion,
			"HandleWebView rpc to directive",
		),
		mux,
		nil,
	)
}

// HandleWebView handles a web view via the HandleWebView directive.
func (h *HandleWebViewViaBus) HandleWebView(
	ctx context.Context,
	req *web_view_handler.HandleWebViewRequest,
) (*web_view_handler.HandleWebViewResponse, error) {
	webView := web_view_client.NewProxyWebViewViaAccess(
		ctx,
		req.GetId(),
		req.GetParentId(),
		req.GetDocumentId(),
		req.GetPermanent(),
		h.accessClient,
	)
	err := web_view.ExHandleWebView(ctx, h.le, h.b, webView, true)
	var errStr string
	if err != nil {
		if err == context.Canceled {
			return nil, err
		}
		errStr = err.Error()
	}
	return &web_view_handler.HandleWebViewResponse{Error: errStr}, nil
}

// _ is a type assertion
var _ web_view_handler.SRPCHandleWebViewServiceServer = ((*HandleWebViewViaBus)(nil))
