package web_runtime

import (
	"context"
	"sort"
	"sync"

	random_id "github.com/aperturerobotics/bldr/util/random-id"
	web_document "github.com/aperturerobotics/bldr/web/document"
	"github.com/aperturerobotics/bldr/web/ipc"
	sw "github.com/aperturerobotics/bldr/web/runtime/sw"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Remote is a remote instance of a WebRuntime.
//
// Communicates with the frontend using bldr/document.ts
type Remote struct {
	runtimeID string
	le        *logrus.Entry
	bus       bus.Bus
	handler   WebRuntimeHandler

	ipc       ipc.IPC
	ipcMux    srpc.Mux
	ipcServer *srpc.Server
	ipcClient srpc.Client

	// swMux services requests for the ServiceWorker.
	swMux srpc.Mux

	// webRuntime is the RPC client for the WebRuntime.
	webRuntime SRPCWebRuntimeClient

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
	// remoteWebDocuments is the current snapshot of web documents.
	// sorted by ID
	// do not retain this slice without holding mtx
	remoteWebDocuments []*RemoteWebDocument
}

// rState contains information about the Remote controller
type rState struct {
	// ctx is the root context for the execute loop
	ctx context.Context
	// webDocuments is the current list of web documents
	webDocuments map[string]*RemoteWebDocument
}

// NewRemote constructs a new browser runtime.
//
// id should be the runtime identifier specified at startup by the js loader.
// initWebDocument should be a handle to the WebDocument which created the Remote.
func NewRemote(le *logrus.Entry, b bus.Bus, handler WebRuntimeHandler, runtimeID string, ipc ipc.IPC) (*Remote, error) {
	if err := ValidateRuntimeId(runtimeID); err != nil {
		return nil, err
	}
	r := &Remote{
		runtimeID: runtimeID,
		le:        le,
		bus:       b,
		handler:   handler,
		ipc:       ipc,

		stateChanged: make(chan struct{}, 1),
		opQueue:      make(chan *remoteOp, 1),
	}

	// WebRuntimeHost mux
	r.ipcMux = srpc.NewMux()
	if err := SRPCRegisterWebRuntimeHost(r.ipcMux, newRemoteWebRuntimeHost(r)); err != nil {
		return nil, err
	}
	r.ipcServer = srpc.NewServer(r.ipcMux)
	r.ipcClient = srpc.NewClientWithMuxedConn(r.ipc)
	r.webRuntime = NewSRPCWebRuntimeClient(r.ipcClient)

	// ServiceWorkerHost mux
	r.swMux = srpc.NewMux()
	if err := sw.SRPCRegisterServiceWorkerHost(r.swMux, sw.NewServiceWorkerHost(r.handler)); err != nil {
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

// GetWebDocuments returns the current snapshot of active WebDocuments.
func (r *Remote) GetWebDocuments(ctx context.Context) (map[string]web_document.WebDocument, error) {
	var out map[string]web_document.WebDocument
	err := r.waitState(ctx, func(s *rState) (bool, error) {
		out = make(map[string]web_document.WebDocument, len(s.webDocuments))
		for webDocumentID, webDocument := range s.webDocuments {
			out[webDocumentID] = webDocument.ctrl.GetWebDocument()
		}
		return false, nil
	})
	return out, err
}

// CreateWebDocument creates a new web document and waits for it to become active.
//
// Returns ErrWebDocumentUnavailable if WebDocument is not available or cannot be created.
func (r *Remote) CreateWebDocument(ctx context.Context, webDocumentID string) (web_document.WebDocument, error) {
	if webDocumentID == "" {
		// generate random id
		webDocumentID = random_id.RandomIdentifier()
	}

	var out web_document.WebDocument
	err := execRemoteOp(ctx, r, func(ctx context.Context, r *Remote) (bool, error) {
		_, rwv := r.lookupRemoteWebDocument(webDocumentID)
		if rwv != nil {
			out = rwv.ctrl.GetWebDocument()
			return false, nil
		}
		_, err := r.webRuntime.CreateWebDocument(ctx, &CreateWebDocumentRequest{
			Id: webDocumentID,
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
		wv, ok := s.webDocuments[webDocumentID]
		if ok && wv != nil {
			out = wv.ctrl.GetWebDocument()
		}
		return out == nil, nil
	})
	return out, err
}

// RemoveWebDocument removes a web document by ID.
// note: this is called by webDocument.Remove.
// returns nil if not found
func (r *Remote) RemoveWebDocument(ctx context.Context, webDocumentID string) error {
	return execRemoteOp(ctx, r, func(ctx context.Context, r *Remote) (bool, error) {
		removedView := r.removeRemoteWebDocument(webDocumentID)
		if removedView == nil {
			return false, nil
		}
		// return true, r.writeRemoveWebDocument(webDocumentID)
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

	// start web document monitoring loop
	initCh := make(chan *WebRuntimeStatus, 1)
	go func() {
		err := r.monitorWebDocuments(ctx, le, initCh)
		if err != nil && err != context.Canceled {
			le.
				WithError(err).
				Warn("monitor web documents exited with error")
		}
		errCh <- err
	}()

	// wait for the initial response from the web document stream. monitorWebDocuments
	// will fetch & process the initial web document list before writing to initCh.
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

// GetWebDocumentMux returns the Mux serving requests for the given WebDocument.
//
// Waits for the given web document ID to be available, or ctx to be canceled.
/*
func (r *Remote) GetWebDocumentMux(ctx context.Context, webDocumentId string) (srpc.Mux, error) {
	var mux srpc.Mux
	err := r.waitState(ctx, func(s *rState) (bool, error) {
		// look for the web document
		_, webDocument := r.lookupRemoteWebDocument(webDocumentId)
		if webDocument != nil {
			mux = webDocument.ctrl.GetWebDocument().(*web_document.Remote).GetMux()
		}
		// keep waiting until mux != nil
		return mux == nil, nil
	})
	if err != nil {
		return nil, err
	}
	return mux, nil
}
*/

// GetWebDocumentOpenStream returns a OpenStreamFunc for the given WebDocument ID.
//
// note: when opening the stream, waits for the given web document to exist.
func (r *Remote) GetWebDocumentOpenStream(webDocumentId string) srpc.OpenStreamFunc {
	return func(ctx context.Context, msgHandler srpc.PacketHandler, closeHandler srpc.CloseHandler) (srpc.Writer, error) {
		// wait for web document to exist
		var webDocument *RemoteWebDocument
		err := r.waitState(ctx, func(s *rState) (bool, error) {
			// look for the web document
			_, webDocument = r.lookupRemoteWebDocument(webDocumentId)
			// keep waiting until web document found
			return webDocument == nil, nil
		})
		if err != nil {
			return nil, err
		}

		// request a stream with the web document
		caller := func(ctx context.Context) (rpcstream.RpcStream, error) {
			return r.webRuntime.WebDocumentRpc(ctx)
		}
		prw, err := rpcstream.OpenRpcStream(ctx, caller, webDocumentId)
		if err != nil {
			return nil, err
		}
		go prw.ReadPump(msgHandler, closeHandler)
		return prw, nil
	}
}

// GetServiceWorkerMux returns the Mux serving requests for the ServiceWorker.
func (r *Remote) GetServiceWorkerMux(ctx context.Context, componentID string) (srpc.Mux, error) {
	// wait for Execute() to be ready
	if err := r.WaitReady(ctx); err != nil {
		return nil, err
	}

	return r.swMux, nil
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
	r.ipc.Close()
	r.mtx.Unlock()

	// TODO: wait for runtime to fully exit.
	return nil
}

// acceptIpcStreamPump is started by Execute and manages accepting streams from ipc.
func (r *Remote) acceptIpcStreamPump(ctx context.Context) error {
	return r.ipcServer.AcceptMuxedConn(ctx, r.ipc)
}

// monitorWebDocuments is started by Execute and manages monitoring web documents.
func (r *Remote) monitorWebDocuments(ctx context.Context, le *logrus.Entry, initialValueCh chan<- *WebRuntimeStatus) error {
	// start a call querying for web documents
	le.Info("starting WebRuntime status monitoring")
	stream, err := r.webRuntime.WatchWebRuntimeStatus(ctx, NewWatchWebRuntimeStatusRequest())
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
			le.Debugf("rx: initial list of %d web documents", len(resp.GetWebDocuments()))
			firstRx = true
			initialValueCh <- resp
		}

		err = execRemoteOp(ctx, r, func(ctx context.Context, r *Remote) (bool, error) {
			return r.handleWebRuntimeStatus(ctx, resp)
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

// handleWebRuntimeStatus handles an incoming web status message.
// expects mtx to be locked
// returns dirty, err
func (r *Remote) handleWebRuntimeStatus(ctx context.Context, ws *WebRuntimeStatus) (bool, error) {
	return r.handleWebDocumentStatuses(ctx, ws.GetSnapshot(), ws.GetWebDocuments())
}

// handleWebDocumentStatuses handles a list of web document statuses.
// snapshot: if set, removes any views that don't appear in the list.
// note: ctx is used as the context for the new remote web document.
// returns dirty, err
// expects mtx to be locked
func (r *Remote) handleWebDocumentStatuses(ctx context.Context, snapshot bool, statuses []*WebDocumentStatus) (bool, error) {
	if !snapshot && len(statuses) == 0 {
		return false, nil
	}

	// notSeenDocs contains web documents /not/ seen in the status list.
	var dirty bool
	notSeenDocs := r.buildCurrentState(ctx).webDocuments
	for _, status := range statuses {
		webDocumentID := status.GetId()
		if webDocumentID == "" {
			continue
		}

		// web document seen: remove from beforeState.
		delete(notSeenDocs, webDocumentID)

		// delete
		if status.GetDeleted() {
			if r.removeRemoteWebDocument(webDocumentID) != nil {
				dirty = true
			}
			continue
		}

		// insert / update
		insertIdx, rwv := r.lookupRemoteWebDocument(webDocumentID)
		if rwv != nil {
			isPermanent := status.GetPermanent()
			if rwv.permanent != isPermanent {
				rwv.permanent = isPermanent
				dirty = true
			}
		} else {
			var err error
			rwv, err = NewRemoteWebDocument(ctx, r, webDocumentID, status.GetPermanent())
			if err != nil {
				// only happens if the ID is formatted incorrectly
				r.le.WithError(err).Error("skipping invalid web document")
				continue
			}
			r.insertRemoteWebDocument(insertIdx, rwv)
			dirty = true
		}
	}

	// if this is a snapshot, delete any views we didn't see.
	if snapshot {
		for webDocumentID := range notSeenDocs {
			if r.removeRemoteWebDocument(webDocumentID) != nil {
				dirty = true
			}
		}
	}

	return dirty, nil
}

// insertRemoteWebDocument adds a new remote web document to the set.
// expects mtx to be locked
func (r *Remote) insertRemoteWebDocument(insertIdx int, doc *RemoteWebDocument) {
	r.remoteWebDocuments = append(r.remoteWebDocuments, nil)
	copy(r.remoteWebDocuments[insertIdx+1:], r.remoteWebDocuments[insertIdx:])
	r.remoteWebDocuments[insertIdx] = doc
	r.le.
		WithField("document-id", doc.id).
		WithField("document-permanent", doc.permanent).
		WithField("document-count", len(r.remoteWebDocuments)).
		Debug("added remote web document")
	go r.handler.HandleWebDocument(doc.ctrl.GetWebDocument())
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
	webDocuments := make(map[string]*RemoteWebDocument, len(r.remoteWebDocuments))
	for _, rwv := range r.remoteWebDocuments {
		webDocuments[rwv.id] = rwv
	}
	return &rState{
		ctx:          ctx,
		webDocuments: webDocuments,
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

// WaitReady waits for the state to not be nil.
func (r *Remote) WaitReady(ctx context.Context) error {
	return r.waitState(ctx, func(s *rState) (bool, error) {
		return false, nil
	})
}

// WaitFirstWebDocument waits for at least one WebDocument to exist.
func (r *Remote) WaitFirstWebDocument(ctx context.Context) (web_document.WebDocument, error) {
	var webDocument web_document.WebDocument
	err := r.waitState(ctx, func(s *rState) (bool, error) {
		for _, wv := range s.webDocuments {
			webDocument = wv.ctrl.GetWebDocument()
			break
		}
		return webDocument == nil, nil
	})
	if err != nil {
		return nil, err
	}
	return webDocument, nil
}

// GetWebDocumentMux returns the Mux serving requests for the given WebDocument.
//
// Waits for the given web view ID to be available, or ctx to be canceled.
func (r *Remote) GetWebDocumentMux(ctx context.Context, webDocumentId string) (srpc.Mux, error) {
	var mux srpc.Mux
	err := r.waitState(ctx, func(s *rState) (bool, error) {
		// look for the web view
		_, webDocument := r.lookupRemoteWebDocument(webDocumentId)
		if webDocument != nil {
			mux = webDocument.remote.GetMux()
		}
		// keep waiting until mux != nil
		return mux == nil, nil
	})
	if err != nil {
		return nil, err
	}
	return mux, nil
}

// wakeExecution wakes the Execution loop.
func (r *Remote) wakeExecution() {
	select {
	case r.wakeExecute <- struct{}{}:
	default:
	}
}

// removeRemoteWebDocument removes a remote web document, if found.
// returns val, error, returns nil, nil if not found
// expects mtx to be locked
func (r *Remote) removeRemoteWebDocument(id string) *RemoteWebDocument {
	idx, doc := r.lookupRemoteWebDocument(id)
	if doc == nil {
		return nil
	}

	// remove idx from the remoteWebDocuments slice
	return r.removeRemoteWebDocumentAtIdx(idx)
}

// removeRemoteWebDocumentAtIdx removes a remote web document at the given index.
func (r *Remote) removeRemoteWebDocumentAtIdx(idx int) *RemoteWebDocument {
	if idx < 0 || idx >= len(r.remoteWebDocuments) {
		return nil
	}

	doc := r.remoteWebDocuments[idx]
	id := doc.id
	r.le.
		WithField("document-id", id).
		Debug("removed remote web document")
	r.remoteWebDocuments = r.remoteWebDocuments[:idx+copy(r.remoteWebDocuments[idx:], r.remoteWebDocuments[idx+1:])]
	return doc
}

// lookupRemoteWebDocument searches the remoteWebDocuments field for a web document.
// returns insertion index if not found
// expects mtx to be locked
func (r *Remote) lookupRemoteWebDocument(id string) (int, *RemoteWebDocument) {
	i := sort.Search(len(r.remoteWebDocuments), func(i int) bool {
		return r.remoteWebDocuments[i].id >= id
	})
	var rwv *RemoteWebDocument
	if i < len(r.remoteWebDocuments) && r.remoteWebDocuments[i].id == id {
		rwv = r.remoteWebDocuments[i]
	}
	return i, rwv
}

// sortRemoteWebDocuments sorts the remoteWebDocuments field.
// expects mtx to be locked
/*
func (r *Remote) sortRemoteWebDocuments() {
	sort.Slice(r.remoteWebDocuments, func(i, j int) bool {
		return r.remoteWebDocuments[i].id < r.remoteWebDocuments[j].id
	})
}
*/

// _ is a type assertion
var (
	_ WebRuntime = ((*Remote)(nil))

	_ rpcstream.RpcStreamGetter = ((*Remote)(nil)).GetWebDocumentMux
	_ rpcstream.RpcStreamGetter = ((*Remote)(nil)).GetServiceWorkerMux
)
