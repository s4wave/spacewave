package web_view

import "errors"

var (
	// ErrEmptyWebViewID is returned if the web view id was empty.
	ErrEmptyWebViewID = errors.New("empty web view id")
	// ErrWebViewUnavailable is returned if WebView is not available
	ErrWebViewUnavailable = errors.New("creating WebViews is unavailable")
	// ErrWebViewPermanent is returned if WebView cannot be closed.
	ErrWebViewPermanent = errors.New("WebView cannot be closed")
)
