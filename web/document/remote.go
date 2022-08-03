package web_document

import (
	"context"
	"sort"

	"github.com/aperturerobotics/bldr/util/cstate"
	random_id "github.com/aperturerobotics/bldr/util/random-id"
	web_view "github.com/aperturerobotics/bldr/web/document/view"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"

	"github.com/sirupsen/logrus"
)

// RemoteVersion is the Version of the web_document.Remote implementation.
var RemoteVersion = semver.MustParse("0.0.1")

// Remote is a remote instance of a WebDocument.
//
// Communicates with the frontend using bldr/web-document.ts
type Remote struct {
	documentID string
	le         *logrus.Entry
	bus        bus.Bus
	handler    WebDocumentHandler

	rpcMux    srpc.Mux
	rpcServer *srpc.Server
	rpcClient srpc.Client

	// webDocument is the RPC client for the WebDocument.
	webDocument SRPCWebDocumentClient

	// cstate is the controller state
	// contains a mutex which guards below fields
	cstate *cstate.CState[*Remote]
	// ready indicates the initial snapshot has been received.
	ready bool
	// remoteWebViews is the current snapshot of web views.
	// sorted by ID
	// do not retain this slice without holding mtx
	remoteWebViews []*RemoteWebView
}

// NewRemote constructs a new browser runtime.
//
// id should be the runtime identifier specified at startup by the js loader.
// initWebView should be a handle to the WebView which created the Remote.
func NewRemote(
	le *logrus.Entry,
	b bus.Bus,
	handler WebDocumentHandler,
	webDocumentId string,
	openStream srpc.OpenStreamFunc,
) (*Remote, error) {
	if err := ValidateWebDocumentId(webDocumentId); err != nil {
		return nil, err
	}

	r := &Remote{
		documentID: webDocumentId,
		le:         le,
		bus:        b,
		handler:    handler,
	}
	r.cstate = cstate.NewCState(r)
	r.rpcMux = srpc.NewMux()
	if err := SRPCRegisterWebDocumentHost(r.rpcMux, newRemoteWebDocumentHost(r)); err != nil {
		return nil, err
	}
	r.rpcServer = srpc.NewServer(r.rpcMux)
	r.rpcClient = srpc.NewClient(openStream)
	r.webDocument = NewSRPCWebDocumentClient(r.rpcClient)

	return r, nil
}

// GetWebDocumentUuid returns the web document identifier.
func (r *Remote) GetWebDocumentUuid() string {
	return r.documentID
}

// GetMux returns the WebDocumentHost service mux.
func (r *Remote) GetMux() srpc.Mux {
	return r.rpcMux
}

// GetLogger returns the root log entry.
func (r *Remote) GetLogger() *logrus.Entry {
	return r.le
}

// GetBus returns the root controller bus to use in this process.
func (r *Remote) GetBus() bus.Bus {
	return r.bus
}

// GetWebViews returns the current snapshot of active WebViews.
func (r *Remote) GetWebViews(ctx context.Context) (map[string]web_view.WebView, error) {
	var out map[string]web_view.WebView
	err := r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
		if !val.ready {
			return false, nil
		}

		out = r.buildRemoteWebViewsMap()
		return true, nil
	})
	return out, err
}

// GetWebView waits for the remote to be ready & returns the given WebView.
// If wait is set, waits for the web document ID to exist.
// Otherwise, returns nil, nil if not found.
func (r *Remote) GetWebView(ctx context.Context, webViewID string, wait bool) (web_view.WebView, error) {
	var out web_view.WebView
	err := r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
		if !val.ready {
			return false, nil
		}

		_, rdoc := r.lookupRemoteWebView(webViewID)
		if rdoc == nil {
			return !wait, nil
		}
		out = rdoc
		return true, nil
	})
	return out, err
}

// WaitReady waits for the state to be ready.
func (r *Remote) WaitReady(ctx context.Context) error {
	return r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
		return r.ready, nil
	})
}

// WaitFirstWebView waits for at least one WebView to exist.
func (r *Remote) WaitFirstWebView(ctx context.Context) (web_view.WebView, error) {
	var webView web_view.WebView
	err := r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
		if !r.ready {
			return false, nil
		}
		for _, wv := range r.remoteWebViews {
			webView = wv
			if webView != nil {
				return true, nil
			}
		}
		return false, nil
	})
	return webView, err
}

// CreateWebView creates a new web view and waits for it to become active.
//
// Returns ErrWebViewUnavailable if WebView is not available or cannot be created.
func (r *Remote) CreateWebView(ctx context.Context, webViewID string) (bool, error) {
	if webViewID == "" {
		// generate random id
		webViewID = random_id.RandomIdentifier()
	}
	return r.cstate.Apply(ctx, func(ctx context.Context, v *cstate.CStateWriter[*Remote]) (dirty bool, err error) {
		_, rwv := r.lookupRemoteWebView(webViewID)
		if rwv != nil {
			return false, nil
		}
		_, err = r.webDocument.CreateWebView(ctx, &CreateWebViewRequest{
			Id: webViewID,
		})
		return err == nil, err
	})
}

// RemoveWebView removes a web view by ID.
// note: this is called by webView.Remove.
func (r *Remote) RemoveWebView(ctx context.Context, webViewID string) (removed bool, err error) {
	return r.cstate.Apply(ctx, func(ctx context.Context, v *cstate.CStateWriter[*Remote]) (dirty bool, err error) {
		_, view := r.lookupRemoteWebView(webViewID)
		if view.permanent {
			return false, ErrWebViewPermanent
		}
		req := &RemoveWebViewRequest{Id: webViewID}
		if res, err := r.webDocument.RemoveWebView(ctx, req); err != nil || !res.GetRemoved() {
			return false, err
		}
		removedView := r.removeRemoteWebView(webViewID)
		return removedView != nil, nil
	})
}

// Execute executes the runtime.
// Returns any errors, nil if Execute is not required.
func (r *Remote) Execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// start stream accept pump
	le := r.le.WithField("document-id", r.documentID)

	// start web view monitoring loop
	errCh := make(chan error, 1)
	go func() {
		err := r.monitorWebViews(ctx, le)
		if err != nil && err != context.Canceled {
			le.
				WithError(err).
				Warn("monitor web views exited with error")
		}
		errCh <- err
	}()

	return r.cstate.Execute(ctx, errCh)
}

// GetWebDocumentMux returns the Mux serving requests for the given WebDocument.
//
// immediately returns a loopback reference to the root Mux.
func (r *Remote) GetWebDocumentMux(ctx context.Context, webDocumentId string) (srpc.Mux, error) {
	if err := r.WaitReady(ctx); err != nil {
		return nil, err
	}

	return r.rpcMux, nil
}

// GetWebViewMux returns the Mux serving requests for the given WebView.
//
// Waits for the given web view ID to be available, or ctx to be canceled.
func (r *Remote) GetWebViewMux(ctx context.Context, webViewId string) (srpc.Mux, error) {
	var mux srpc.Mux
	err := r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
		if !r.ready {
			return false, nil
		}
		_, doc := r.lookupRemoteWebView(webViewId)
		if doc == nil {
			return false, nil
		}
		mux = doc.mux
		return mux != nil, nil
	})
	return mux, err
}

// GetWebViewOpenStream returns a OpenStreamFunc for the given WebView ID.
//
// note: when opening the stream, waits for the given web document to exist.
func (r *Remote) GetWebViewOpenStream(webViewId string) srpc.OpenStreamFunc {
	return func(ctx context.Context, msgHandler srpc.PacketHandler, closeHandler srpc.CloseHandler) (srpc.Writer, error) {
		return r.WebViewOpenStream(ctx, msgHandler, closeHandler, webViewId)
	}
}

// WebViewOpenStream opens a stream with the given WebView ID.
//
// note: when opening the stream, waits for the given web document to exist.
func (r *Remote) WebViewOpenStream(
	ctx context.Context,
	msgHandler srpc.PacketHandler,
	closeHandler srpc.CloseHandler,
	webViewID string,
) (srpc.Writer, error) {
	var writer srpc.Writer
	err := r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
		if !r.ready {
			return false, nil
		}
		// wait for web document to exist
		_, doc := r.lookupRemoteWebView(webViewID)
		if doc == nil {
			return false, nil
		}
		// request a stream with the web document
		caller := func(ctx context.Context) (rpcstream.RpcStream, error) {
			return r.webDocument.WebViewRpc(ctx)
		}
		prw, err := rpcstream.OpenRpcStream(ctx, caller, webViewID)
		if err != nil {
			return false, err
		}
		go prw.ReadPump(msgHandler, closeHandler)
		writer = prw
		return true, nil
	})
	return writer, err
}

// monitorWebViews is started by Execute and manages monitoring web views.
func (r *Remote) monitorWebViews(ctx context.Context, le *logrus.Entry) error {
	// start a call querying for web views
	le.Info("starting WebDocument status monitoring")
	defer le.Info("stopped WebDocument status monitoring")

	stream, err := r.webDocument.WatchWebDocumentStatus(ctx, NewWatchWebDocumentStatusRequest())
	if err != nil {
		return err
	}

	var firstRx bool
	for {
		// ensure context is not canceled
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-stream.Context().Done():
			return context.Canceled
		default:
		}

		resp, err := stream.Recv()
		if err != nil {
			return err
		}

		if !firstRx {
			le.Debugf("rx: initial list of %d web views", len(resp.GetWebViews()))
			firstRx = true
		}

		r.cstate.Apply(ctx, func(ctx context.Context, v *cstate.CStateWriter[*Remote]) (dirty bool, err error) {
			return r.handleWebStatus(ctx, resp)
		})
		if err != nil {
			return err
		}
	}
}

// handleWebStatus handles an incoming web status message.
// expects mtx to be locked
// returns dirty, err
func (r *Remote) handleWebStatus(ctx context.Context, ws *WebDocumentStatus) (bool, error) {
	return r.handleWebViewStatuses(ctx, ws.GetSnapshot(), ws.GetWebViews())
}

// handleWebViewStatuses handles a list of web view statuses.
// snapshot: if set, removes any views that don't appear in the list.
// note: ctx is used as the context for the new remote web view.
// returns dirty, err
// expects mtx to be locked
func (r *Remote) handleWebViewStatuses(ctx context.Context, snapshot bool, statuses []*WebViewStatus) (bool, error) {
	if !snapshot && len(statuses) == 0 {
		return false, nil
	}

	// we got a snapshot or initial list of statuses: mark as ready
	r.ready = true

	// notSeenViews contains web views /not/ seen in the status list.
	var dirty bool
	notSeenViews := r.buildRemoteWebViewsMap()
	for _, status := range statuses {
		webViewID := status.GetId()
		if webViewID == "" {
			continue
		}

		// web view seen: remove from beforeState.
		delete(notSeenViews, webViewID)

		// delete
		if status.GetDeleted() {
			if r.removeRemoteWebView(webViewID) != nil {
				dirty = true
			}
			continue
		}

		// insert / update
		insertIdx, rwv := r.lookupRemoteWebView(webViewID)
		if rwv != nil {
			isPermanent := status.GetPermanent()
			if rwv.permanent != isPermanent {
				rwv.permanent = isPermanent
				dirty = true
			}
		} else {
			rwv = NewRemoteWebView(ctx, r, webViewID, status.GetPermanent())
			r.insertRemoteWebView(insertIdx, rwv)
			dirty = true
		}
	}

	// if this is a snapshot, delete any views we didn't see.
	if snapshot {
		for webViewID := range notSeenViews {
			if r.removeRemoteWebView(webViewID) != nil {
				dirty = true
			}
		}
	}

	return dirty, nil
}

// insertRemoteWebView adds a new remote web view to the set.
// expects mtx to be locked
func (r *Remote) insertRemoteWebView(insertIdx int, rwv *RemoteWebView) {
	r.remoteWebViews = append(r.remoteWebViews, nil)
	copy(r.remoteWebViews[insertIdx+1:], r.remoteWebViews[insertIdx:])
	r.remoteWebViews[insertIdx] = rwv
	r.le.
		WithField("view-id", rwv.id).
		WithField("view-permanent", rwv.permanent).
		WithField("view-count", len(r.remoteWebViews)).
		Debug("added remote web view")
	go r.handler.HandleWebView(rwv)
}

// buildRemoteWebViewsMap builds the mapping of ID to WebDocument.
// expects mtx to be locked.
func (r *Remote) buildRemoteWebViewsMap() map[string]web_view.WebView {
	out := make(map[string]web_view.WebView, len(r.remoteWebViews))
	for _, webView := range r.remoteWebViews {
		out[webView.id] = webView
	}
	return out
}

// removeRemoteWebView removes a remote web view and returns its final status, if found.
// returns val, error, returns nil, nil if not found
// expects mtx to be locked
func (r *Remote) removeRemoteWebView(id string) *RemoteWebView {
	idx, rwv := r.lookupRemoteWebView(id)
	if rwv == nil {
		return nil
	}

	// remove idx from the remoteWebViews slice
	r.le.WithField("view-id", id).Debug("removed remote web view")
	r.remoteWebViews = r.remoteWebViews[:idx+copy(r.remoteWebViews[idx:], r.remoteWebViews[idx+1:])]
	return rwv
}

// lookupRemoteWebView searches the remoteWebViews field for a web view.
// returns insertion index if not found
// expects mtx to be locked
func (r *Remote) lookupRemoteWebView(id string) (int, *RemoteWebView) {
	i := sort.Search(len(r.remoteWebViews), func(i int) bool {
		return r.remoteWebViews[i].id >= id
	})
	var rwv *RemoteWebView
	if i < len(r.remoteWebViews) && r.remoteWebViews[i].id == id {
		rwv = r.remoteWebViews[i]
	}
	return i, rwv
}

// sortRemoteWebViews sorts the remoteWebViews field.
// expects mtx to be locked
/*
func (r *Remote) sortRemoteWebViews() {
	sort.Slice(r.remoteWebViews, func(i, j int) bool {
		return r.remoteWebViews[i].id < r.remoteWebViews[j].id
	})
}
*/

// _ is a type assertion
var (
	_ WebDocument = ((*Remote)(nil))

	_ rpcstream.RpcStreamGetter = ((*Remote)(nil)).GetWebViewMux
)
