package web_runtime

// WebView is a HTML/CSS/JavaScript environment (browser window).
//
// Scripts, assets, and raw HTML snippets can be mounted into the view.
// Other abstractions for shadow-dom and dependency management are implemented.
type WebView interface {
	// TODO manage css/html/scripts
	// TODO mount paths to the service worker

	// Close shuts down the WebView and closes the window/tab if possible.
	// Returns ErrWebViewPermanent if the view cannot be closed.
	// Note: browser windows not created by CreateWebView cannot be closed.
	Close() error
}
