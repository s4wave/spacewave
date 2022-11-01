package web_view

import (
	context "context"

	srpc "github.com/aperturerobotics/starpc/srpc"
)

// WebView is a HTML/CSS/JavaScript container.
//
// Scripts, assets, and raw HTML snippets can be mounted into the view.
type WebView interface {
	// TODO manage css/html/scripts

	// GetId returns the web view identifier.
	GetId() string

	// GetParentId returns the id of the parent web view (if any)
	GetParentId() string

	// GetPermanent returns if the web view is not removable.
	GetPermanent() bool

	// GetClient returns the SRPC client for the remote WebView and other services.
	GetClient() srpc.Client

	// SetRenderMode updates the RenderMode of the WebView.
	SetRenderMode(ctx context.Context, req *SetRenderModeRequest) (*SetRenderModeResponse, error)

	// Remove shuts down the WebView and closes the window/tab if possible.
	// Returns ErrWebViewPermanent if the view cannot be closed.
	// Returns context.Canceled if ctx is canceled (but still processes the op)
	// Note: browser windows not created by CreateWebView cannot be closed.
	Remove(ctx context.Context) error
}
