package web_runtime

import (
	"context"
	"io"
	"sort"
	"sync"
	"time"

	stream_packet "github.com/aperturerobotics/bifrost/stream/packet"
	"github.com/aperturerobotics/bldr/web/ipc"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// maxMessageSize constrains the message buffer allocation size.
// currently set to 2MB
const maxMessageSize = 2000000

// Remote is a remote instance of a web runtime.
//
// Communicates with the frontend using bldr/runtime.ts
type Remote struct {
	runtimeID string
	le        *logrus.Entry
	bus       bus.Bus
	ipc       ipc.IPC
	ipcPkt    *stream_packet.Session

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
	// msgQueue contains queued messages from web views.
	msgQueue []*WebToRuntime
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
	r := &Remote{
		runtimeID: runtimeID,
		le:        le,
		bus:       b,
		ipc:       ipc,

		stateChanged: make(chan struct{}),
		wakeExecute:  make(chan struct{}, 1),
		opQueue:      make(chan *remoteOp),
	}
	r.ipcPkt = stream_packet.NewSession(r.ipc, maxMessageSize)
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
		return false, r.writeCreateWebView(webViewID)
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
		return true, r.writeRemoveWebView(webViewID)
	})
}

// Execute executes the runtime.
// Returns any errors, nil if Execute is not required.
func (r *Remote) Execute(ctx context.Context) error {
	le := r.le.WithField("runtime-id", r.runtimeID)

	// start read pump
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.readIPCMessages(ctx)
	}()

	// write query view status
	// views will announce when they are created & when queried.
	le.Infof("web runtime starting up: querying for web views")
	if err := r.writeQueryViewStatus(); err != nil {
		return err
	}

	// wait a moment to collect responses
	// if the responses take longer than this, they will be added later.
	select {
	case <-ctx.Done():
	case <-time.After(time.Millisecond * 120):
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

		// process message queue
		msgs := r.msgQueue
		r.msgQueue = nil
		for _, msg := range msgs {
			procDirty, err := r.processMessage(ctx, msg)
			if err != nil {
				le.WithError(err).Warn("error processing message from remote")
			}
			if procDirty {
				dirty = true
			}
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

// WriteMessage writes a proto message to the stream.
func (r *Remote) WriteMessage(msg *RuntimeToWeb) error {
	return r.ipcPkt.SendMsg(msg)
}

// HandleMessage enqueues a message for processing.
func (r *Remote) HandleMessage(msg *WebToRuntime) {
	r.mtx.Lock()
	r.msgQueue = append(r.msgQueue, msg)
	r.mtx.Unlock()
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
	r.ipcPkt.Close()
	r.mtx.Unlock()

	// TODO: wait for runtime to fully exit.
	return nil
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

// readIPCMessages reads & parses messages coming from the IPC.
func (r *Remote) readIPCMessages(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		default:
		}

		msg := &WebToRuntime{}
		if err := r.ipcPkt.RecvMsg(msg); err != nil {
			if err == context.Canceled || err == io.EOF {
				return err
			}
			r.le.WithError(err).Warn("ignoring recvmsg error")
			continue
		}

		if msg.GetMessageType() != 0 {
			r.HandleMessage(msg)
		}
	}
}

// processMessage processes a message from the WebViews in Execute.
// note: mtx is locked by caller
// returns dirty, err
// called by Execute
func (r *Remote) processMessage(ctx context.Context, msg *WebToRuntime) (bool, error) {
	r.le.Infof("processing message: %v", msg)
	switch msg.GetMessageType() {
	case WebToRuntimeType_WebToRuntimeType_WEB_STATUS:
		return true, r.handleWebStatus(ctx, msg.GetWebStatus())
	}
	return false, errors.Errorf("unhandled message type: %v", msg.GetMessageType().String())
}

// handleWebStatus handles an incoming web status message.
// expects mtx to be locked
func (r *Remote) handleWebStatus(ctx context.Context, ws *WebStatus) error {
	return r.handleWebViewStatuses(ctx, ws.GetSnapshot(), ws.GetWebViews())
}

// handleWebViewStatuses handles a list of web view statuses.
// snapshot: if set, removes any views that don't appear in the list.
// note: ctx is used as the context for the new remote web view.
// expects mtx to be locked
func (r *Remote) handleWebViewStatuses(ctx context.Context, snapshot bool, statuses []*WebViewStatus) error {
	if !snapshot && len(statuses) == 0 {
		return nil
	}

	// notSeenViews contains web views /not/ seen in the status list.
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
			_ = r.removeRemoteWebView(webViewID)
			continue
		}

		// insert / update
		insertIdx, rwv := r.lookupRemoteWebView(webViewID)
		if rwv != nil {
			rwv.permanent = status.GetPermanent()
		} else {
			rwv = NewRemoteWebView(ctx, r, webViewID, status.GetPermanent())
			r.insertRemoteWebView(insertIdx, rwv)
		}
	}

	// if this is a snapshot, delete any views we didn't see.
	if snapshot {
		for webViewID := range notSeenViews {
			_ = r.removeRemoteWebView(webViewID)
		}
	}

	return nil
}

// insertRemoteWebView adds a new remote web view to the set.
func (r *Remote) insertRemoteWebView(insertIdx int, rwv *RemoteWebView) {
	r.le.
		WithField("view-id", rwv.id).
		WithField("view-permanent", rwv.permanent).
		Debug("added remote web view")
	r.remoteWebViews = append(r.remoteWebViews, nil)
	copy(r.remoteWebViews[insertIdx+1:], r.remoteWebViews[insertIdx:])
	r.remoteWebViews[insertIdx] = rwv
}

// writeQueryViewStatus writes the QueryViewStatus command.
func (r *Remote) writeQueryViewStatus() error {
	return r.WriteMessage(NewQueryWebStatus())
}

// writeCreateWebView sends a message to the runtime to create a new web view at the root of the tree.
// usually this means to create a new Window or Tab.
func (r *Remote) writeCreateWebView(webViewID string) error {
	return r.WriteMessage(&RuntimeToWeb{
		MessageType: RuntimeToWebType_RuntimeToWebType_CREATE_VIEW,
		CreateView: &CreateView{
			Id: webViewID,
		},
	})
}

// writeRemoveWebView sends a message to the runtime to remove the web view.
// note: if the web view was permanent, the remote runtime will reject the remove command.
func (r *Remote) writeRemoveWebView(webViewID string) error {
	return r.WriteMessage(&RuntimeToWeb{
		MessageType: RuntimeToWebType_RuntimeToWebType_REMOVE_VIEW,
		RemoveView: &RemoveView{
			Id: webViewID,
		},
	})
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
		case r.wakeExecute <- struct{}{}:
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
var _ WebRuntime = ((*Remote)(nil))
