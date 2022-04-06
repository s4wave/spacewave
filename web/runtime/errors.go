package web_runtime

import "errors"

var (
	// ErrWebViewUnavailable is returned if WebView is not available
	ErrWebViewUnavailable = errors.New("web view is unavailable in this environment")
	// ErrWebViewPermanent is returned if WebView cannot be closed.
	ErrWebViewPermanent = errors.New("web view cannot be closed")
)
