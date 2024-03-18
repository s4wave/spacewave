package web_runtime

import (
	"context"
	"io"
	"slices"
	"sort"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	random_id "github.com/aperturerobotics/bifrost/util/randstring"
	"github.com/aperturerobotics/bldr/util/cstate"
	web_document "github.com/aperturerobotics/bldr/web/document"
	sw "github.com/aperturerobotics/bldr/web/runtime/sw"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
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

	rpcMux    srpc.Mux
	rpcServer *srpc.Server
	rpcClient srpc.Client

	execListener func(ctx context.Context, r *Remote) error

	// swMux services requests for the ServiceWorker.
	swMux srpc.Mux

	// webRuntime is the RPC client for the WebRuntime.
	webRuntime SRPCWebRuntimeClient

	// snapshotCtr contains a snapshot of below state for observers.
	// it is not a source of truth and is only for GetWebRuntimeStatus
	// contains nil until the remote is ready or closed
	snapshotCtr *ccontainer.CContainer[*WebRuntimeStatus]

	// cstate is the controller state
	// contains a mutex which guards below fields
	cstate *cstate.CState[*Remote]
	// ready indicates the initial snapshot has been received.
	ready bool
	// closed indicates the remote runtime is closed.
	closed bool
	// remoteWebDocuments is the current snapshot of web documents.
	// sorted by ID
	// do not retain this slice without holding mtx
	remoteWebDocuments []*RemoteWebDocument
}

// NewRemote constructs a new browser runtime.
//
// id should be the runtime identifier specified at startup by the js loader.
// initWebDocument should be a handle to the WebDocument which created the Remote.
func NewRemote(
	le *logrus.Entry,
	b bus.Bus,
	handler WebRuntimeHandler,
	runtimeID string,
	rpcClient srpc.Client,
	execListener func(ctx context.Context, r *Remote) error,
) (*Remote, error) {
	if err := ValidateRuntimeId(runtimeID); err != nil {
		return nil, err
	}

	r := &Remote{
		runtimeID:    runtimeID,
		le:           le,
		bus:          b,
		handler:      handler,
		rpcClient:    rpcClient,
		execListener: execListener,
	}

	r.cstate = cstate.NewCState(r)
	r.snapshotCtr = ccontainer.NewCContainerVT[*WebRuntimeStatus](nil)
	_, _ = r.cstate.AddWatcher(context.Background(), false, func(ctx context.Context, state *Remote) {
		r.updateStatusSnapshot()
	})

	// WebRuntimeHost mux
	r.rpcMux = srpc.NewMux()
	if err := SRPCRegisterWebRuntimeHost(r.rpcMux, newRemoteWebRuntimeHost(r)); err != nil {
		return nil, err
	}
	r.rpcServer = srpc.NewServer(r.rpcMux)
	r.webRuntime = NewSRPCWebRuntimeClient(r.rpcClient)

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

// GetRpcServer returns the RPC server.
func (r *Remote) GetRpcServer() *srpc.Server {
	return r.rpcServer
}

// GetWebRuntimeStatusCtr contains a full snapshot of the web runtime status.
func (r *Remote) GetWebRuntimeStatusCtr() *ccontainer.CContainer[*WebRuntimeStatus] {
	return r.snapshotCtr
}

// GetWebDocuments returns the current snapshot of active WebDocuments.
func (r *Remote) GetWebDocuments(ctx context.Context) (map[string]web_document.WebDocument, error) {
	var out map[string]web_document.WebDocument
	err := r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
		if !val.ready {
			return false, nil
		}

		out = r.buildRemoteWebDocumentsMap()
		return true, nil
	})
	return out, err
}

// GetWebDocument waits for the remote to be ready & returns the given WebDocument.
// If wait is set, waits for the web document ID to exist.
// Otherwise, returns nil, nil if not found.
func (r *Remote) GetWebDocument(ctx context.Context, webDocumentID string, wait bool) (web_document.WebDocument, error) {
	var out web_document.WebDocument
	err := r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
		if !val.ready {
			return false, nil
		}

		_, rdoc := r.lookupRemoteWebDocument(webDocumentID)
		if rdoc == nil {
			return !wait, nil
		}
		out = rdoc.remote
		return true, nil
	})
	return out, err
}

// CreateWebDocument creates a new web document.
//
// Returns created, error: returns false for created if already exists.
// Returns false, nil if WebDocument cannot be created.
func (r *Remote) CreateWebDocument(ctx context.Context, webDocumentID string) (bool, error) {
	if webDocumentID == "" {
		// generate random id
		webDocumentID = random_id.RandomIdentifier(8)
	}
	return r.cstate.Apply(ctx, func(ctx context.Context, v *cstate.CStateWriter[*Remote]) (dirty bool, err error) {
		_, rwv := r.lookupRemoteWebDocument(webDocumentID)
		if rwv != nil {
			return false, nil
		}
		_, err = r.webRuntime.CreateWebDocument(ctx, &CreateWebDocumentRequest{
			Id: webDocumentID,
		})
		return err == nil, err
	})
}

// RemoveWebDocument removes a web document by ID.
// note: this is called by webDocument.Remove.
// returns nil if not found
func (r *Remote) RemoveWebDocument(ctx context.Context, webDocumentID string) (removed bool, err error) {
	return r.cstate.Apply(ctx, func(ctx context.Context, v *cstate.CStateWriter[*Remote]) (dirty bool, err error) {
		req := &RemoveWebDocumentRequest{Id: webDocumentID}
		if res, err := r.webRuntime.RemoveWebDocument(ctx, req); err != nil || !res.GetRemoved() {
			return false, err
		}
		removedDoc := r.removeRemoteWebDocument(webDocumentID)
		return removedDoc != nil, nil
	})
}

// Execute executes the runtime.
// Returns any errors, nil if Execute is not required.
func (r *Remote) Execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	le := r.le.WithField("runtime-id", r.runtimeID)
	errCh := make(chan error, 2)

	// start incoming stream listener, if applicable
	if r.execListener != nil {
		go func() {
			err := r.execListener(ctx, r)
			if err != nil && err != context.Canceled {
				le.
					WithError(err).
					Warn("listen for streams exited with error")
			}
			errCh <- err
		}()
	}

	// start web document monitoring loop
	go func() {
		err := r.monitorWebDocuments(ctx, le)
		if err != nil && err != context.Canceled {
			le.
				WithError(err).
				Warn("monitor web documents exited with error")
		}
		errCh <- err
	}()

	// execute the event & operation loop
	return r.cstate.Execute(ctx, errCh)
}

// GetWebDocumentHost returns the Mux serving requests for the given WebDocument.
//
// Waits for the given web document ID to be available, or ctx to be canceled.
/*
func (r *Remote) GetWebDocumentHost(ctx context.Context, webDocumentID string) (srpc.Mux, error) {
	var mux srpc.Mux
	err := r.waitState(ctx, func(s *rState) (bool, error) {
		// look for the web document
		_, webDocument := r.lookupRemoteWebDocument(webDocumentID)
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
func (r *Remote) GetWebDocumentOpenStream(webDocumentID string) srpc.OpenStreamFunc {
	return func(ctx context.Context, msgHandler srpc.PacketDataHandler, closeHandler srpc.CloseHandler) (srpc.PacketWriter, error) {
		return r.WebDocumentOpenStream(ctx, msgHandler, closeHandler, webDocumentID)
	}
}

// WebDocumentOpenStream opens a stream with the given WebDocument ID.
//
// note: when opening the stream, waits for the given web document to exist.
func (r *Remote) WebDocumentOpenStream(
	ctx context.Context,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
	webDocumentID string,
) (srpc.PacketWriter, error) {
	var writer srpc.PacketWriter
	err := r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
		if !r.ready {
			return false, nil
		}
		// wait for web document to exist
		_, doc := r.lookupRemoteWebDocument(webDocumentID)
		if doc == nil {
			return false, nil
		}
		// request a stream with the web document
		rw, err := rpcstream.OpenRpcStream(ctx, r.webRuntime.WebDocumentRpc, webDocumentID, false)
		if err != nil {
			return false, err
		}
		go rpcstream.ReadPump(rw, msgHandler, closeHandler)
		writer = rpcstream.NewRpcStreamWriter(rw)
		return true, nil
	})
	return writer, err
}

// GetWebWorkerOpenStream returns a OpenStreamFunc for the given WebWorker ID.
//
// note: when opening the stream, waits for the given web worker to exist.
func (r *Remote) GetWebWorkerOpenStream(webWorkerID string) srpc.OpenStreamFunc {
	return func(ctx context.Context, msgHandler srpc.PacketDataHandler, closeHandler srpc.CloseHandler) (srpc.PacketWriter, error) {
		return r.WebWorkerOpenStream(ctx, msgHandler, closeHandler, webWorkerID)
	}
}

// WebWorkerOpenStream opens a stream with the given WebDocument ID.
//
// note: when opening the stream, waits for the given web document to exist.
func (r *Remote) WebWorkerOpenStream(
	ctx context.Context,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
	webWorkerID string,
) (srpc.PacketWriter, error) {
	var writer srpc.PacketWriter
	err := r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
		if !r.ready {
			return false, nil
		}
		// request a stream with the web worker
		rw, err := rpcstream.OpenRpcStream(ctx, r.webRuntime.WebWorkerRpc, webWorkerID, false)
		if err != nil {
			return false, err
		}
		go rpcstream.ReadPump(rw, msgHandler, closeHandler)
		writer = rpcstream.NewRpcStreamWriter(rw)
		return true, nil
	})
	return writer, err
}

// GetServiceWorkerHost returns the Invoker serving requests for the ServiceWorker.
func (r *Remote) GetServiceWorkerHost(ctx context.Context, componentID string) (srpc.Invoker, func(), error) {
	// wait for Execute() to be ready
	if err := r.WaitReady(ctx); err != nil {
		return nil, nil, err
	}

	return r.swMux, nil, nil
}

// monitorWebDocuments is started by Execute and manages monitoring web documents.
func (r *Remote) monitorWebDocuments(ctx context.Context, le *logrus.Entry) error {
	// start a call querying for web documents
	le.Debug("starting WebRuntime status monitoring")
	defer le.Debug("stopped WebRuntime status monitoring")

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
			if ctx.Err() != nil {
				return context.Canceled
			}
			return io.EOF
		default:
		}

		resp, err := stream.Recv()
		if err != nil {
			return err
		}

		if !firstRx {
			le.Debugf("rx: initial list of %d web documents", len(resp.GetWebDocuments()))
			firstRx = true
		}

		_, err = r.cstate.Apply(ctx, func(ctx context.Context, v *cstate.CStateWriter[*Remote]) (dirty bool, err error) {
			return r.handleWebRuntimeStatus(ctx, resp)
		})
		if err != nil {
			le.WithError(err).Warn("rx: error processing web runtime status")
			return err
		}
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

	// we got a snapshot or initial list of statuses: mark as ready
	r.ready = true

	// notSeenDocs contains web documents /not/ seen in the status list.
	var dirty bool
	notSeenDocs := r.buildRemoteWebDocumentsMap()
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
	r.remoteWebDocuments = slices.Insert(r.remoteWebDocuments, insertIdx, doc)
	r.le.
		WithField("document-id", doc.id).
		WithField("document-permanent", doc.permanent).
		WithField("document-count", len(r.remoteWebDocuments)).
		Debug("added remote web document")
	r.handler.HandleWebDocument(doc.ctrl.GetWebDocument())
}

// WaitReady waits for the state to be ready.
func (r *Remote) WaitReady(ctx context.Context) error {
	return r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
		return r.ready, nil
	})
}

// WaitFirstWebDocument waits for at least one WebDocument to exist.
func (r *Remote) WaitFirstWebDocument(ctx context.Context) (web_document.WebDocument, error) {
	var webDocument web_document.WebDocument
	err := r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
		if !r.ready {
			return false, nil
		}
		for _, wv := range r.remoteWebDocuments {
			webDocument = wv.ctrl.GetWebDocument()
			if webDocument != nil {
				return true, nil
			}
		}
		return false, nil
	})
	return webDocument, err
}

// GetWebDocumentHost returns the Mux serving requests for the given WebDocument.
//
// Waits for the given web view ID to be available, or ctx to be canceled.
func (r *Remote) GetWebDocumentHost(ctx context.Context, webDocumentID string) (srpc.Invoker, func(), error) {
	var mux srpc.Mux
	err := r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
		if !r.ready {
			return false, nil
		}
		_, doc := r.lookupRemoteWebDocument(webDocumentID)
		if doc == nil {
			return false, nil
		}
		mux = doc.remote.GetMux()
		return mux != nil, nil
	})
	return mux, nil, err
}

// GetWebWorkerHost returns the Mux serving requests for the given WebWorker.
//
// Waits for the given web worker ID to be available, or ctx to be canceled.
func (r *Remote) GetWebWorkerHost(ctx context.Context, webWorkerID string) (srpc.Invoker, func(), error) {
	// TODO: Separate mux for each WebWorker?
	// TODO server ID should be plugin/[plugin-id]
	/*
		var mux srpc.Mux
		err := r.cstate.Wait(ctx, func(ctx context.Context, val *Remote) (bool, error) {
			if !r.ready {
				return false, nil
			}
			_, doc := r.lookupRemoteWebDocument(webDocumentID)
			if doc == nil {
				return false, nil
			}
			mux = doc.remote.GetMux()
			return mux != nil, nil
		})
		return mux, nil, err
	*/
	return bifrost_rpc.NewInvoker(r.bus, "web-worker/"+webWorkerID, true), nil, nil
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
	rdoc := r.removeRemoteWebDocumentAtIdx(idx)
	if rdoc != nil {
		rdoc.Close()
	}
	return rdoc
}

// removeRemoteWebDocumentAtIdx removes a remote web document at the given index.
func (r *Remote) removeRemoteWebDocumentAtIdx(idx int) *RemoteWebDocument {
	if idx < 0 || idx >= len(r.remoteWebDocuments) {
		return nil
	}

	doc := r.remoteWebDocuments[idx]
	id := doc.id
	r.remoteWebDocuments = append(r.remoteWebDocuments[:idx], r.remoteWebDocuments[idx+1:]...)
	r.le.
		WithField("document-id", id).
		WithField("document-count", len(r.remoteWebDocuments)).
		Debug("removed remote web document")
	return doc
}

// buildRemoteWebDocumentsMap builds the mapping of ID to WebDocument.
// expects mtx to be locked.
func (r *Remote) buildRemoteWebDocumentsMap() map[string]web_document.WebDocument {
	out := make(map[string]web_document.WebDocument, len(r.remoteWebDocuments))
	for _, webDocument := range r.remoteWebDocuments {
		out[webDocument.id] = webDocument.ctrl.GetWebDocument()
	}
	return out
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

// updateStatusSnapshot generates the status snapshot and sets it in the container.
// expects mtx to be locked
func (r *Remote) updateStatusSnapshot() {
	if !r.ready && !r.closed {
		return
	}
	status := &WebRuntimeStatus{
		Snapshot: true,
		Closed:   r.closed,
	}
	if r.ready && !r.closed {
		for _, remoteWebDoc := range r.remoteWebDocuments {
			status.WebDocuments = append(status.WebDocuments, &WebDocumentStatus{
				Id:        remoteWebDoc.id,
				Permanent: remoteWebDoc.permanent,
			})
		}
	}
	r.snapshotCtr.SetValue(status)
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

	_ rpcstream.RpcStreamGetter = ((*Remote)(nil)).GetWebDocumentHost
	_ rpcstream.RpcStreamGetter = ((*Remote)(nil)).GetServiceWorkerHost
)
