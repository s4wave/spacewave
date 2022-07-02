package web_runtime

import (
	"context"
	"sort"
	"sync"

	"github.com/aperturerobotics/bldr/web/ipc"
	sw "github.com/aperturerobotics/bldr/web/runtime/sw"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"

	"github.com/libp2p/go-libp2p-core/network"
	p2pmplex "github.com/libp2p/go-libp2p/p2p/muxer/mplex"
	mplex "github.com/libp2p/go-mplex"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Remote is a remote instance of a web runtime.
//
// Communicates with the frontend using bldr/runtime.ts
type Remote struct {
	runtimeID string
	le        *logrus.Entry
	bus       bus.Bus

	ipc       ipc.IPC
	ipcMplex  network.MuxedConn
	ipcMux    srpc.Mux
	ipcServer *srpc.Server
	ipcClient srpc.Client

	// webRuntime is the RPC client for the WebRuntime.
	webRuntime SRPCWebRuntimeClient

	// swMux is the mux with services for the ServiceWorker to call.
	swMux srpc.Mux

	// stateChanged is written to when state changes
	stateChanged chan struct{}
	// wakeExecute wakes execute when any below fields changes
	wakeExecute chan struct{}
	// opQueue is pushed to wake Execute to perform an operation.
	// used whenever mtx needs to be locked
	opQueue chan *remoteOp
	// mtx guards below fields
	mtx sync.Mutex
	// state contains the current state, or nil if not resolved
	state *rState
	// stateCtx is canceled when state is changed
	// nil if state == nil
	stateCtx context.Context
	// stateCtxCancel cancels stateCtx
	stateCtxCancel context.CancelFunc
	// remoteWebViews is the current snapshot of web views.
	// sorted by ID
	// do not retain this slice without holding mtx
	remoteWebViews []*RemoteWebView
}

// rState contains information about the Remote controller
type rState struct {
	// ctx is the root context for the execute loop
	ctx context.Context
	// webViews is the current list of web views
	webViews map[string]*RemoteWebView
}

// NewRemote constructs a new browser runtime.
//
// id should be the runtime identifier specified at startup by the js loader.
// initWebView should be a handle to the WebView which created the Remote.
func NewRemote(le *logrus.Entry, b bus.Bus, runtimeID string, ipc ipc.IPC) (*Remote, error) {
	if err := ValidateRuntimeId(runtimeID); err != nil {
		return nil, err
	}
	r := &Remote{
		runtimeID: runtimeID,
		le:        le,
		bus:       b,
		ipc:       ipc,

		stateChanged: make(chan struct{}, 1),
		wakeExecute:  make(chan struct{}, 1),
		opQueue:      make(chan *remoteOp, 1),
	}
	ipcMplex, err := mplex.NewMultiplex(r.ipc, false, nil)
	if err != nil {
		return nil, err
	}
	r.ipcMplex = p2pmplex.NewMuxedConn(ipcMplex)
	r.ipcMux = srpc.NewMux()
	if err := SRPCRegisterHostRuntime(r.ipcMux, newRemoteHostRuntime(r)); err != nil {
		return nil, err
	}
	r.ipcServer = srpc.NewServer(r.ipcMux)
	r.ipcClient = srpc.NewClientWithMuxedConn(r.ipcMplex)
	r.webRuntime = NewSRPCWebRuntimeClient(r.ipcClient)
	r.swMux = srpc.NewMux()
	if err := sw.SRPCRegisterServiceWorkerHost(r.swMux, newRemoteServiceWorkerHost(r)); err != nil {
		return nil, err
	}
	return r, nil
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
func (r *Remote) GetWebViews(ctx context.Context) (map[string]WebView, error) {
	var out map[string]WebView
	err := r.waitState(ctx, func(s *rState) (bool, error) {
		out = make(map[string]WebView, len(s.webViews))
		for webViewID, webView := range s.webViews {
			out[webViewID] = webView
		}
		return false, nil
	})
	return out, err
}

// CreateWebView creates a new web view and waits for it to become active.
//
// Returns ErrWebViewUnavailable if WebView is not available or cannot be created.
func (r *Remote) CreateWebView(ctx context.Context, webViewID string) (WebView, error) {
	if webViewID == "" {
		// generate random id
		webViewID = randomIdentifier()
	}

	var out WebView
	err := execRemoteOp(ctx, r, func(ctx context.Context, r *Remote) (bool, error) {
		_, rwv := r.lookupRemoteWebView(webViewID)
		if rwv != nil {
			out = rwv
			return false, nil
		}
		_, err := r.webRuntime.CreateWebView(ctx, &CreateWebViewRequest{
			Id: webViewID,
		})
		if err != nil {
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return out, err
	}

	err = r.waitState(ctx, func(s *rState) (bool, error) {
		wv, ok := s.webViews[webViewID]
		if ok && wv != nil {
			out = wv
		}
		return out == nil, nil
	})
	return out, err
}

// RemoveWebView removes a web view by ID.
// note: this is called by webView.Remove.
// returns nil if not found
func (r *Remote) RemoveWebView(ctx context.Context, webViewID string) error {
	return execRemoteOp(ctx, r, func(ctx context.Context, r *Remote) (bool, error) {
		removedView := r.removeRemoteWebView(webViewID)
		if removedView == nil {
			return false, nil
		}
		// return true, r.writeRemoveWebView(webViewID)
		return true, errors.New("TODO")
	})
}

// Execute executes the runtime.
// Returns any errors, nil if Execute is not required.
func (r *Remote) Execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// start stream accept pump
	le := r.le.WithField("runtime-id", r.runtimeID)
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.acceptIpcStreamPump(ctx)
	}()

	// start web view monitoring loop
	initCh := make(chan *WebStatus, 1)
	go func() {
		errCh <- r.monitorWebViews(ctx, le, initCh)
	}()

	// wait for the initial response from the web view stream. monitorWebViews
	// will fetch & process the initial web view list before writing to initCh.
	select {
	case <-ctx.Done():
		return context.Canceled
	case <-initCh:
	}

	var dirty bool
	for {
		// lock mtx
		r.mtx.Lock()

		// flush wakeExecute channel
		select {
		case <-r.wakeExecute:
		default:
		}

		processOp := func(op *remoteOp) (dirty bool, err error) {
			if op.opFn != nil {
				dirty, err = op.opFn(ctx, r)
			}
			return dirty, err
		}

		// process op queue
		for {
			var op *remoteOp
			select {
			case op = <-r.opQueue:
			default:
			}
			if op == nil {
				break
			}
			// mark op with result
			opDirty, err := processOp(op)
			if err == nil && opDirty {
				dirty = true
			}
			op.finish(err)
		}

		// write the state
		if dirty {
			rs := r.buildCurrentState(ctx)
			r.pushState(rs)
		}

		// unlock
		r.mtx.Unlock()

		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-errCh:
			return err
		case <-r.wakeExecute:
		}
	}
}

// GetWebRuntimeMux returns the Mux serving requests for the given WebRuntime.
//
// immediately returns a loopback reference to the root Mux.
func (r *Remote) GetWebRuntimeMux(ctx context.Context, webRuntimeId string) (srpc.Mux, error) {
	r.le.Infof("DEBUG: get web runtime mux: waiting for ready: %s", webRuntimeId)
	if err := r.waitReady(ctx); err != nil {
		return nil, err
	}
	r.le.Infof("DEBUG: get web runtime mux: wait ready complete: %s", webRuntimeId)

	return r.ipcMux, nil
}

// GetServiceWorkerMux returns the Mux serving requests for the ServiceWorker.
func (r *Remote) GetServiceWorkerMux(ctx context.Context, componentID string) (srpc.Mux, error) {
	// expect component id "sw" always.
	if componentID != "sw" {
		return nil, errors.New("unexpected component id")
	}

	// wait for Execute() to be ready
	if err := r.waitReady(ctx); err != nil {
		return nil, err
	}

	return r.swMux, nil
}

// GetWebViewMux returns the Mux serving requests for the given WebView.
//
// Waits for the given web view ID to be available, or ctx to be canceled.
func (r *Remote) GetWebViewMux(ctx context.Context, webViewId string) (srpc.Mux, error) {
	var mux srpc.Mux
	err := r.waitState(ctx, func(s *rState) (bool, error) {
		// look for the web view
		_, webView := r.lookupRemoteWebView(webViewId)
		if webView != nil {
			mux = webView.GetMux()
		}
		// keep waiting until mux != nil
		return mux == nil, nil
	})
	if err != nil {
		return nil, err
	}
	return mux, nil
}

// GetWebViewOpenStream returns a OpenStreamFunc for the given WebView ID.
//
// note: when opening the stream, waits for the given web view id to exist.
func (r *Remote) GetWebViewOpenStream(webViewId string) srpc.OpenStreamFunc {
	return func(ctx context.Context, msgHandler srpc.PacketHandler) (srpc.Writer, error) {
		// wait for web view to exist
		var webView *RemoteWebView
		err := r.waitState(ctx, func(s *rState) (bool, error) {
			// look for the web view
			_, webView = r.lookupRemoteWebView(webViewId)
			// keep waiting until web view found
			return webView == nil, nil
		})
		if err != nil {
			return nil, err
		}

		// request a stream with the web view
		caller := func(ctx context.Context) (rpcstream.RpcStream, error) {
			return r.webRuntime.WebViewRpc(ctx)
		}
		prw, err := rpcstream.OpenRpcStream(ctx, caller, webViewId)
		if err != nil {
			return nil, err
		}
		go func() {
			_ = prw.ReadPump(msgHandler)
		}()
		return prw, nil
	}
}

// Close closes the runtime and waits for Execute to finish if ctx is provided
func (r *Remote) Close(ctx context.Context) error {
	// close all windows
	r.mtx.Lock()
	r.state = nil
	if r.stateCtxCancel != nil {
		r.stateCtxCancel()
		r.stateCtx, r.stateCtxCancel = nil, nil
	}
	r.ipcMplex.Close()
	r.mtx.Unlock()

	// TODO: wait for runtime to fully exit.
	return nil
}

// acceptIpcStreamPump is started by Execute and manages accepting streams from ipc.
func (r *Remote) acceptIpcStreamPump(ctx context.Context) error {
	return r.ipcServer.AcceptMuxedConn(ctx, r.ipcMplex)
}

// monitorWebViews is started by Execute and manages monitoring web views.
func (r *Remote) monitorWebViews(ctx context.Context, le *logrus.Entry, initialValueCh chan<- *WebStatus) error {
	// start a call querying for web views
	le.Info("starting web status monitoring")
	stream, err := r.webRuntime.WatchWebStatus(ctx, NewWatchWebStatusRequest())
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
			initialValueCh <- resp
		}

		err = execRemoteOp(ctx, r, func(ctx context.Context, r *Remote) (bool, error) {
			return r.handleWebStatus(ctx, resp)
		})
		if err != nil {
			return err
		}
	}
}

// waitRemoteOp queues & waits for a remoteOp to complete.
func (r *Remote) waitRemoteOp(op *remoteOp) error {
	ctx := op.ctx
	select {
	case <-ctx.Done():
		return context.Canceled
	case r.opQueue <- op:
		r.wakeExecution()
	}
	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-op.resCh:
		return err
	}
}

// handleWebStatus handles an incoming web status message.
// expects mtx to be locked
// returns dirty, err
func (r *Remote) handleWebStatus(ctx context.Context, ws *WebStatus) (bool, error) {
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

	// notSeenViews contains web views /not/ seen in the status list.
	var dirty bool
	notSeenViews := r.buildCurrentState(ctx).webViews
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
func (r *Remote) insertRemoteWebView(insertIdx int, rwv *RemoteWebView) {
	r.remoteWebViews = append(r.remoteWebViews, nil)
	copy(r.remoteWebViews[insertIdx+1:], r.remoteWebViews[insertIdx:])
	r.remoteWebViews[insertIdx] = rwv
	r.le.
		WithField("view-id", rwv.id).
		WithField("view-permanent", rwv.permanent).
		WithField("view-count", len(r.remoteWebViews)).
		Debug("added remote web view")
}

// pushState triggers all waiters.
// expects mtx to be locked
func (r *Remote) pushState(s *rState) {
	if r.stateCtxCancel != nil {
		r.stateCtxCancel()
	}
	r.state = s
	r.stateCtx, r.stateCtxCancel = context.WithCancel(s.ctx)
	for {
		select {
		case r.stateChanged <- struct{}{}:
		default:
			return
		}
	}
}

// buildCurrentState builds a rState for the current state.
// expects mtx to be locked
func (r *Remote) buildCurrentState(ctx context.Context) *rState {
	webViews := make(map[string]*RemoteWebView, len(r.remoteWebViews))
	for _, rwv := range r.remoteWebViews {
		webViews[rwv.id] = rwv
	}
	return &rState{
		ctx:      ctx,
		webViews: webViews,
	}
}

// waitState waits for state to not be nil & the callback to return false
func (r *Remote) waitState(ctx context.Context, cb func(s *rState) (bool, error)) error {
	for {
		var cntu bool
		var err error
		r.mtx.Lock()
		st := r.state
		if st != nil {
			cntu, err = cb(st)
		}
		r.mtx.Unlock()
		if err != nil || (st != nil && !cntu) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.stateChanged:
		}
	}
}

// waitReady waits for the state to not be nil.
func (r *Remote) waitReady(ctx context.Context) error {
	return r.waitState(ctx, func(s *rState) (bool, error) {
		return false, nil
	})
}

// wakeExecution wakes the Execution loop.
func (r *Remote) wakeExecution() {
	select {
	case r.wakeExecute <- struct{}{}:
	default:
	}
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
func (r *Remote) sortRemoteWebViews() {
	sort.Slice(r.remoteWebViews, func(i, j int) bool {
		return r.remoteWebViews[i].id < r.remoteWebViews[j].id
	})
}

// _ is a type assertion
var (
	_ WebRuntime = ((*Remote)(nil))

	_ rpcstream.RpcStreamGetter = ((*Remote)(nil)).GetWebViewMux
	_ rpcstream.RpcStreamGetter = ((*Remote)(nil)).GetServiceWorkerMux
)
