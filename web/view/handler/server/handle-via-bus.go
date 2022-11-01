package web_view_handler_server

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/view"
	web_view_client "github.com/aperturerobotics/bldr/web/view/client"
	web_view_handler "github.com/aperturerobotics/bldr/web/view/handler"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/sirupsen/logrus"
)

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
		le: le,
		b:  b,
	}
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
