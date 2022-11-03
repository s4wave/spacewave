package web_view_server

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/view"
)

// WebViewServer provides the WebView SRPC service with a WebView.
type WebViewServer struct {
	// view is the WebView
	view web_view.WebView
}

// NewWebViewServer constructs a WebViewServer from a WebView.
func NewWebViewServer(view web_view.WebView) *WebViewServer {
	return &WebViewServer{view: view}
}

// SetRenderMode changes the render mode of the web view.
func (s *WebViewServer) SetRenderMode(
	ctx context.Context,
	req *web_view.SetRenderModeRequest,
) (*web_view.SetRenderModeResponse, error) {
	return s.view.SetRenderMode(ctx, req)
}

// RemoveWebView removes the web view.
func (s *WebViewServer) RemoveWebView(
	ctx context.Context,
	req *web_view.RemoveWebViewRequest,
) (*web_view.RemoveWebViewResponse, error) {
	err := s.view.Remove(ctx)
	if err != nil {
		if err == web_view.ErrWebViewPermanent {
			return &web_view.RemoveWebViewResponse{Removed: false}, nil
		}
		return nil, err
	}
	return &web_view.RemoveWebViewResponse{Removed: true}, nil
}

// _ is a type assertion
var _ web_view.SRPCWebViewServer = ((*WebViewServer)(nil))
