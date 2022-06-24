package web_runtime

import (
	"context"

	"github.com/aperturerobotics/bifrost/util/randstring"
	"github.com/aperturerobotics/starpc/srpc"
)

// WebView is a HTML/CSS/JavaScript container.
//
// Scripts, assets, and raw HTML snippets can be mounted into the view.
// Other abstractions for shadow-dom and dependency management are implemented.
type WebView interface {
	// TODO manage css/html/scripts
	// TODO mount paths to the service worker

	// GetMux returns the mux for the WebView services.
	GetMux() srpc.Mux

	// Remove shuts down the WebView and closes the window/tab if possible.
	// Returns ErrWebViewPermanent if the view cannot be closed.
	// Returns context.Canceled if ctx is canceled (but still processes the op)
	// Note: browser windows not created by CreateWebView cannot be closed.
	Remove(ctx context.Context) error
}

// randomIdentifier generates a random string identifier.
func randomIdentifier() string {
	return randstring.RandString(nil, 8)
}
