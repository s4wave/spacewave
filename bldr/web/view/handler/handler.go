package web_view_handler

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccall"
	"github.com/aperturerobotics/util/filter"
	"github.com/pkg/errors"
	web_view "github.com/s4wave/spacewave/bldr/web/view"
	"github.com/sirupsen/logrus"
)

// WebViewHandler handles a WebView.
type WebViewHandler func(
	ctx context.Context,
	webView web_view.WebView,
) error

// WebViewHandlerWithFilters wraps a WebViewHandler with string filters.
type WebViewHandlerWithFilters struct {
	// Handler is the wrapped WebViewHandler function.
	Handler WebViewHandler
	// WebViewIdFilter filters by web view id.
	WebViewIdFilter *filter.StringFilter
	// WebViewParentIdFilter filters by web view parent id.
	WebViewParentIdFilter *filter.StringFilter
}

// WebViewHandlersWithFilters wraps multiple handlers with both global and per-handler filters.
type WebViewHandlersWithFilters struct {
	// Handlers is the list of handlers with their individual filters.
	Handlers []WebViewHandlerWithFilters
	// GlobalWebViewIdFilter filters by web view id (applies to all handlers).
	GlobalWebViewIdFilter *filter.StringFilter
	// GlobalWebViewParentIdFilter filters by web view parent id (applies to all handlers).
	GlobalWebViewParentIdFilter *filter.StringFilter
}

// MergeWebViewHandlers merges multiple handlers into a single WebViewHandler.
//
// Calls all handlers concurrently, returns first error.
func MergeWebViewHandlers(handlers ...WebViewHandler) WebViewHandler {
	if len(handlers) == 1 {
		return handlers[0]
	}

	return func(ctx context.Context, webView web_view.WebView) error {
		if len(handlers) == 0 {
			return nil
		}

		var ccallFns []ccall.CallConcurrentlyFunc
		for _, handler := range handlers {
			ccallFns = append(ccallFns, func(ctx context.Context) error {
				return handler(ctx, webView)
			})
		}

		return ccall.CallConcurrently(ctx, ccallFns...)
	}
}

// NewViaBusHandler handles the WebView via the HandleWebView directive.
//
// If returnIfErr is set, returns an error if any of the resolvers fail.
// returnIfErr should be set to true in most cases.
func NewViaBusHandler(le *logrus.Entry, b bus.Bus, returnIfErr bool) WebViewHandler {
	return func(
		ctx context.Context,
		webView web_view.WebView,
	) error {
		return web_view.ExHandleWebView(ctx, le, b, webView, returnIfErr)
	}
}

// NewSetRenderMode builds a new handler that sets the render mode.
//
// le can be nil
func NewSetRenderMode(le *logrus.Entry, req *web_view.SetRenderModeRequest, opts ...func(r *web_view.SetRenderModeRequest)) WebViewHandler {
	for _, opt := range opts {
		opt(req)
	}
	return func(
		ctx context.Context,
		webView web_view.WebView,
	) error {
		if le != nil {
			le = req.Logger(le)
			le.Debug("setting render mode")
		}
		_, err := webView.SetRenderMode(ctx, req)
		return err
	}
}

// SetRenderModeWithRefresh is an option to enable Clear on SetRenderMode.
func SetRenderModeWithRefresh() func(m *web_view.SetRenderModeRequest) {
	return func(r *web_view.SetRenderModeRequest) {
		r.Refresh = true
	}
}

// NewSetReactComponent builds a handler that sets a react component.
//
// le can be empty
func NewSetReactComponent(le *logrus.Entry, scriptPath string, props []byte, opts ...func(r *web_view.SetRenderModeRequest)) WebViewHandler {
	return NewSetRenderMode(le, &web_view.SetRenderModeRequest{
		RenderMode: web_view.RenderMode_RenderMode_REACT_COMPONENT,
		ScriptPath: scriptPath,
		Props:      props,
	}, opts...)
}

// NewSetFunctionComponent builds a handler that sets a function callback component.
//
// le can be empty
// props can be nil (undefined)
func NewSetFunctionComponent(le *logrus.Entry, scriptPath string, props []byte) WebViewHandler {
	return NewSetRenderMode(le, &web_view.SetRenderModeRequest{
		RenderMode: web_view.RenderMode_RenderMode_FUNCTION,
		ScriptPath: scriptPath,
		Props:      props,
	})
}

// NewSetHtmlLinks builds a new handler that sets html links.
//
// le can be nil
func NewSetHtmlLinks(le *logrus.Entry, req *web_view.SetHtmlLinksRequest) WebViewHandler {
	return func(
		ctx context.Context,
		webView web_view.WebView,
	) error {
		if le != nil {
			le = req.Logger(le)
			le.Debug("setting html links")
		}
		_, err := webView.SetHtmlLinks(ctx, req)
		return err
	}
}

// Validate validates the WebViewHandlersConfig.
func (m *WebViewHandlersConfig) Validate() error {
	if err := m.GetWebViewId().Validate(); err != nil {
		return errors.Wrap(err, "web_view_id filter")
	}
	if err := m.GetWebViewParentId().Validate(); err != nil {
		return errors.Wrap(err, "web_view_parent_id filter")
	}

	for i, handler := range m.GetHandlers() {
		if err := handler.Validate(); err != nil {
			return errors.Errorf("handler %d: %v", i, err)
		}
	}

	return nil
}

// Validate validates the WebViewHandlerConfig.
func (m *WebViewHandlerConfig) Validate() error {
	if m.GetHandler() == nil {
		return errors.New("handler cannot be nil")
	}

	switch handler := m.GetHandler().(type) {
	case *WebViewHandlerConfig_SetRenderMode:
		if handler.SetRenderMode.SizeVT() == 0 {
			return errors.New("set_render_mode handler cannot be empty")
		}
	case *WebViewHandlerConfig_SetHtmlLinks:
		if handler.SetHtmlLinks.SizeVT() == 0 {
			return errors.New("set_html_links handler cannot be empty")
		}
	default:
		return errors.New("unknown handler type")
	}

	return nil
}

// NewWebViewHandlersFromConfig constructs WebViewHandlersWithFilters from WebViewHandlersConfig.
func NewWebViewHandlersFromConfig(le *logrus.Entry, config *WebViewHandlersConfig) (*WebViewHandlersWithFilters, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	configHandlers := config.GetHandlers()
	handlers := make([]WebViewHandlerWithFilters, 0, len(configHandlers))

	for _, handlerConfig := range configHandlers {
		handler, err := NewWebViewHandlerFromConfig(le, handlerConfig)
		if err != nil {
			return nil, err
		}

		// Keep per-handler filters separate
		handlerWithFilters := WebViewHandlerWithFilters{
			Handler:               handler,
			WebViewIdFilter:       handlerConfig.GetWebViewId(),
			WebViewParentIdFilter: handlerConfig.GetWebViewParentId(),
		}

		handlers = append(handlers, handlerWithFilters)
	}

	return &WebViewHandlersWithFilters{
		Handlers:                    handlers,
		GlobalWebViewIdFilter:       config.GetWebViewId(),
		GlobalWebViewParentIdFilter: config.GetWebViewParentId(),
	}, nil
}

// NewWebViewHandlerFromConfig constructs a WebViewHandler from WebViewHandlerConfig.
func NewWebViewHandlerFromConfig(le *logrus.Entry, config *WebViewHandlerConfig) (WebViewHandler, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	switch handler := config.GetHandler().(type) {
	case *WebViewHandlerConfig_SetRenderMode:
		return NewSetRenderMode(le, handler.SetRenderMode), nil
	case *WebViewHandlerConfig_SetHtmlLinks:
		return NewSetHtmlLinks(le, handler.SetHtmlLinks), nil
	default:
		return nil, errors.New("unknown handler type")
	}
}

// CheckMatch checks if the handler matches the given web view ID and parent ID.
func (h *WebViewHandlerWithFilters) CheckMatch(webViewId, webViewParentId string) bool {
	// Check handler-specific filters
	if !h.WebViewIdFilter.CheckMatch(webViewId) {
		return false
	}
	if !h.WebViewParentIdFilter.CheckMatch(webViewParentId) {
		return false
	}
	return true
}

// CheckMatch checks if the global filters match the given web view ID and parent ID,
// and that at least one handler also matches.
func (h *WebViewHandlersWithFilters) CheckMatch(webViewId, webViewParentId string) bool {
	// Check global filters
	if !h.GlobalWebViewIdFilter.CheckMatch(webViewId) {
		return false
	}
	if !h.GlobalWebViewParentIdFilter.CheckMatch(webViewParentId) {
		return false
	}

	// Check that at least one handler matches
	for _, handler := range h.Handlers {
		if handler.CheckMatch(webViewId, webViewParentId) {
			return true
		}
	}
	return false
}

// GetMatchingHandlers returns handlers from WebViewHandlersWithFilters that match the given web view ID and parent ID.
func (h *WebViewHandlersWithFilters) GetMatchingHandlers(webViewId, webViewParentId string) []WebViewHandler {
	// Check global filters first
	if !h.CheckMatch(webViewId, webViewParentId) {
		return nil
	}

	var matchingHandlers []WebViewHandler
	for _, handler := range h.Handlers {
		if handler.CheckMatch(webViewId, webViewParentId) {
			matchingHandlers = append(matchingHandlers, handler.Handler)
		}
	}

	return matchingHandlers
}

// GetMatchingHandler returns the handler from WebViewHandlerWithFilters if it matches the given web view ID and parent ID.
func (h *WebViewHandlerWithFilters) GetMatchingHandler(webViewId, webViewParentId string) WebViewHandler {
	if h.CheckMatch(webViewId, webViewParentId) {
		return h.Handler
	}
	return nil
}
