package web_view

import "errors"

var (
	// ErrWebViewUnavailable is returned if WebView is not available
	ErrWebViewUnavailable = errors.New("creating WebViews is unavailable")
	// ErrWebViewPermanent is returned if WebView cannot be closed.
	ErrWebViewPermanent = errors.New("WebView cannot be closed")
)
