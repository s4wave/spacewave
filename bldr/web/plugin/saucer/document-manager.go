package saucer

import (
	"context"
	"encoding/binary"
	"io"
	"maps"
	"net/http"
	"strings"
	"sync"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	"github.com/sirupsen/logrus"
)

// documentState tracks the yamux mux connection for a single web document.
type documentState struct {
	id        string
	connected bool

	// mux is the yamux mux connection to JS.
	// Set when JS connects via /b/saucer/{docId}/mux GET.
	mux srpc.MuxedConn

	// mc is the underlying muxConn for posting data from JS.
	mc *muxConn
}

// DocumentManager tracks connected web documents and manages RPC streams
// via yamux multiplexed over a single HTTP streaming connection per document.
type DocumentManager struct {
	le *logrus.Entry

	// bcast guards docs and defaultDocID and broadcasts on changes.
	bcast broadcast.Broadcast
	docs  map[string]*documentState

	// defaultDocID is the document ID for incoming Go->JS streams.
	defaultDocID string

	// snapshotCtr contains the current WebRuntimeStatus snapshot.
	snapshotCtr *ccontainer.CContainer[*web_runtime.WebRuntimeStatus]

	// server is the SRPC server for handling JS-initiated RPC streams.
	server *srpc.Server
}

// NewDocumentManager constructs a new DocumentManager.
func NewDocumentManager(le *logrus.Entry) *DocumentManager {
	return &DocumentManager{
		le:          le,
		docs:        make(map[string]*documentState),
		snapshotCtr: ccontainer.NewCContainerVT[*web_runtime.WebRuntimeStatus](nil),
	}
}

// SetServer sets the SRPC server for handling JS-initiated RPC streams.
func (dm *DocumentManager) SetServer(srv *srpc.Server) {
	dm.server = srv
}

// GetWebRuntimeStatusCtr returns the status container.
func (dm *DocumentManager) GetWebRuntimeStatusCtr() *ccontainer.CContainer[*web_runtime.WebRuntimeStatus] {
	return dm.snapshotCtr
}

// getOrCreateDoc returns or creates a document state.
func (dm *DocumentManager) getOrCreateDoc(id string) *documentState {
	var d *documentState
	dm.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		var ok bool
		d, ok = dm.docs[id]
		if !ok {
			d = &documentState{id: id}
			dm.docs[id] = d
		}
	})
	return d
}

// updateStatusSnapshot updates the WebRuntimeStatus snapshot.
func (dm *DocumentManager) updateStatusSnapshot() {
	var status *web_runtime.WebRuntimeStatus
	dm.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		status = &web_runtime.WebRuntimeStatus{Snapshot: true}
		for _, doc := range dm.docs {
			if doc.connected {
				status.WebDocuments = append(status.WebDocuments, &web_runtime.WebDocumentStatus{
					Id:        doc.id,
					Permanent: true,
				})
			}
		}
	})
	dm.snapshotCtr.SetValue(status)
}

// Close closes all documents.
func (dm *DocumentManager) Close() {
	var docs map[string]*documentState
	dm.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		docs = dm.docs
		dm.docs = make(map[string]*documentState)
	})
	for _, d := range docs {
		if d.mux != nil {
			_ = d.mux.Close()
		}
	}
}

// ServeSaucerHTTP handles /b/saucer/* routes.
func (dm *DocumentManager) ServeSaucerHTTP(rw http.ResponseWriter, req *http.Request) {
	path := req.URL.Path

	docID, remainder, ok := parseSaucerPath(path)
	if !ok {
		rw.WriteHeader(404)
		_, _ = rw.Write([]byte("invalid saucer path"))
		return
	}

	switch remainder {
	case "mux":
		switch req.Method {
		case "GET":
			dm.handleMuxRead(rw, req, docID)
		case "POST":
			dm.handleMuxWrite(rw, req, docID)
		default:
			rw.WriteHeader(405)
		}
	default:
		rw.WriteHeader(404)
		_, _ = rw.Write([]byte("unknown saucer route"))
	}
}

// muxConn bridges the mux read (GET streaming response) and mux write (POST data)
// into a single io.ReadWriteCloser for yamux.
type muxConn struct {
	ctx    context.Context
	cancel context.CancelFunc

	// writeCh receives data from JS (POST bodies) to be read by yamux.
	writeCh chan []byte

	// flushCh receives data from yamux to be written to the JS response.
	flushCh chan []byte

	// pending holds leftover data from a previous writeCh read.
	pendingMu sync.Mutex
	pending   []byte
}

// Read returns data posted by JS to the mux write endpoint.
func (mc *muxConn) Read(p []byte) (int, error) {
	mc.pendingMu.Lock()
	if len(mc.pending) > 0 {
		n := copy(p, mc.pending)
		mc.pending = mc.pending[n:]
		if len(mc.pending) == 0 {
			mc.pending = nil
		}
		mc.pendingMu.Unlock()
		return n, nil
	}
	mc.pendingMu.Unlock()

	select {
	case <-mc.ctx.Done():
		return 0, mc.ctx.Err()
	case data, ok := <-mc.writeCh:
		if !ok {
			return 0, io.EOF
		}
		n := copy(p, data)
		if n < len(data) {
			mc.pendingMu.Lock()
			mc.pending = data[n:]
			mc.pendingMu.Unlock()
		}
		return n, nil
	}
}

// Write sends data from yamux to JS via the streaming GET response.
func (mc *muxConn) Write(p []byte) (int, error) {
	data := make([]byte, len(p))
	copy(data, p)
	select {
	case mc.flushCh <- data:
		return len(p), nil
	case <-mc.ctx.Done():
		return 0, mc.ctx.Err()
	}
}

// Close closes the mux connection.
func (mc *muxConn) Close() error {
	mc.cancel()
	return nil
}

// handleMuxRead handles GET /b/saucer/{docId}/mux.
// This is a long-lived streaming response that carries yamux frames from Go to JS.
func (dm *DocumentManager) handleMuxRead(rw http.ResponseWriter, req *http.Request, docID string) {
	doc := dm.getOrCreateDoc(docID)

	// Close old mux if reconnecting (page reload).
	dm.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if doc.mux != nil {
			_ = doc.mux.Close()
			doc.mux = nil
		}
		doc.connected = false
		broadcast()
	})

	muxCtx, muxCancel := context.WithCancel(req.Context())
	mc := &muxConn{
		ctx:     muxCtx,
		cancel:  muxCancel,
		writeCh: make(chan []byte, 64),
		flushCh: make(chan []byte, 64),
	}

	// JS is inbound (connects to us), we are outbound (open streams to JS).
	// In yamux terms: JS=client, Go=server. So Go side is outbound=false.
	yamuxConn, err := srpc.NewMuxedConnWithRwc(muxCtx, mc, false, nil)
	if err != nil {
		muxCancel()
		dm.le.WithError(err).Error("failed to create yamux mux conn")
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte("yamux init failed"))
		return
	}

	// Register the mux and mark connected.
	dm.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		doc.mux = yamuxConn
		doc.mc = mc
		doc.connected = true
		dm.defaultDocID = docID
		broadcast()
	})
	dm.updateStatusSnapshot()
	dm.le.WithField("doc-id", docID).Debug("document mux connected")

	// Accept JS-initiated streams in the background.
	go dm.acceptMuxStreams(muxCtx, yamuxConn)

	// Stream yamux output to JS via the HTTP response.
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Content-Type", "application/octet-stream")
	rw.WriteHeader(200)

	for {
		select {
		case <-muxCtx.Done():
			dm.disconnectDoc(doc)
			return
		case <-req.Context().Done():
			muxCancel()
			dm.disconnectDoc(doc)
			return
		case data, ok := <-mc.flushCh:
			if !ok {
				dm.disconnectDoc(doc)
				return
			}
			if _, err := rw.Write(data); err != nil {
				dm.le.WithField("doc-id", docID).WithError(err).Debug("mux read write error")
				muxCancel()
				dm.disconnectDoc(doc)
				return
			}
		}
	}
}

// handleMuxWrite handles POST /b/saucer/{docId}/mux.
// Receives yamux frames from JS and queues them for the mux reader.
func (dm *DocumentManager) handleMuxWrite(rw http.ResponseWriter, req *http.Request, docID string) {
	// Wait for the mux connection to be ready.
	// JS may POST before the GET handler finishes creating the muxConn.
	var mc *muxConn
	for {
		var ch <-chan struct{}
		dm.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			if doc, ok := dm.docs[docID]; ok && doc.connected {
				mc = doc.mc
			}
		})
		if mc != nil {
			break
		}
		select {
		case <-req.Context().Done():
			return
		case <-ch:
		}
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte("failed to read body"))
		return
	}

	if len(body) == 0 {
		rw.WriteHeader(204)
		return
	}

	select {
	case mc.writeCh <- body:
		rw.WriteHeader(204)
	case <-mc.ctx.Done():
		rw.WriteHeader(503)
		_, _ = rw.Write([]byte("mux closed"))
	case <-req.Context().Done():
		return
	}
}

// disconnectDoc marks a document as disconnected and cleans up.
func (dm *DocumentManager) disconnectDoc(doc *documentState) {
	dm.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if doc.mux != nil {
			_ = doc.mux.Close()
			doc.mux = nil
		}
		doc.mc = nil
		doc.connected = false
		broadcast()
	})
	dm.updateStatusSnapshot()
}

// acceptMuxStreams accepts yamux streams from JS and routes them to the SRPC server.
func (dm *DocumentManager) acceptMuxStreams(ctx context.Context, mc srpc.MuxedConn) {
	for {
		stream, err := mc.AcceptStream()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			dm.le.WithError(err).Debug("mux accept stream error")
			return
		}
		if dm.server == nil {
			dm.le.Error("SRPC server not set, cannot handle JS-initiated stream")
			_ = stream.Close()
			continue
		}
		go dm.server.HandleStream(ctx, stream)
	}
}

// WebDocumentOpenStream opens an RPC stream with the given WebDocument via yamux.
func (dm *DocumentManager) WebDocumentOpenStream(
	ctx context.Context,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
	webDocumentID string,
) (srpc.PacketWriter, error) {
	doc := dm.waitForDoc(ctx, webDocumentID)
	if doc == nil {
		return nil, ctx.Err()
	}

	var mc srpc.MuxedConn
	dm.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		mc = doc.mux
	})
	if mc == nil {
		return nil, errors.New("document mux not connected")
	}

	stream, err := mc.OpenStream(ctx)
	if err != nil {
		return nil, err
	}

	// Wrap the yamux stream as an SRPC packet stream.
	// Use direct framing: length-prefixed SRPC packets.
	bridge := &yamuxStreamBridge{stream: stream}
	go func() {
		var pumpErr error
		var count int
		for {
			data, err := bridge.RecvRaw()
			if err != nil {
				pumpErr = err
				break
			}
			if len(data) == 0 {
				continue
			}
			count++
			if err = msgHandler(data); err != nil {
				pumpErr = err
				break
			}
		}
		dm.le.
			WithField("packets", count).
			WithError(pumpErr).
			Debug("WebDocumentOpenStream: read pump exited")
		if closeHandler != nil {
			closeHandler(pumpErr)
		}
	}()

	return &yamuxPacketWriter{ctx: ctx, bridge: bridge}, nil
}

// yamuxStreamBridge wraps a yamux stream with length-prefix framing.
type yamuxStreamBridge struct {
	stream io.ReadWriteCloser
	readMu sync.Mutex
}

// RecvRaw reads a length-prefixed frame from the yamux stream.
func (b *yamuxStreamBridge) RecvRaw() ([]byte, error) {
	b.readMu.Lock()
	defer b.readMu.Unlock()

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(b.stream, lenBuf); err != nil {
		return nil, err
	}
	msgLen := binary.LittleEndian.Uint32(lenBuf)
	if msgLen > MaxFrameSize {
		return nil, io.ErrShortBuffer
	}
	data := make([]byte, msgLen)
	if _, err := io.ReadFull(b.stream, data); err != nil {
		return nil, err
	}
	return data, nil
}

// SendRaw writes a length-prefixed frame to the yamux stream.
func (b *yamuxStreamBridge) SendRaw(data []byte) error {
	lenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBuf, uint32(len(data))) //nolint:gosec
	if _, err := b.stream.Write(lenBuf); err != nil {
		return err
	}
	_, err := b.stream.Write(data)
	return err
}

// Close closes the underlying yamux stream.
func (b *yamuxStreamBridge) Close() error {
	return b.stream.Close()
}

// yamuxPacketWriter writes SRPC packets over a yamux stream with length-prefix framing.
type yamuxPacketWriter struct {
	ctx    context.Context
	bridge *yamuxStreamBridge
}

// WritePacket writes a packet to the remote.
func (w *yamuxPacketWriter) WritePacket(p *srpc.Packet) error {
	data, err := p.MarshalVT()
	if err != nil {
		return err
	}
	return w.bridge.SendRaw(data)
}

// Close signals that the writer will no longer send data.
func (w *yamuxPacketWriter) Close() error {
	return nil
}

// Context returns the stream context.
func (w *yamuxPacketWriter) Context() context.Context {
	return w.ctx
}

// _ is a type assertion
var _ srpc.PacketWriter = ((*yamuxPacketWriter)(nil))

// waitForDoc waits for a document to exist and be connected with a mux.
func (dm *DocumentManager) waitForDoc(ctx context.Context, docID string) *documentState {
	for {
		var doc *documentState
		var ch <-chan struct{}
		dm.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			doc = dm.docs[docID]
		})

		if doc != nil && doc.connected && doc.mux != nil {
			return doc
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ch:
		}
	}
}

// parseSaucerPath parses /b/saucer/{docId}/{remainder}.
func parseSaucerPath(path string) (docID, remainder string, ok bool) {
	prefix := "/b/saucer/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	rest := path[len(prefix):]
	before, after, ok0 := strings.Cut(rest, "/")
	if !ok0 {
		return rest, "", rest != ""
	}
	docID = before
	remainder = after
	return docID, remainder, docID != ""
}

// GetWebDocuments returns the current snapshot of active WebDocuments.
func (dm *DocumentManager) GetWebDocuments() map[string]*documentState {
	var out map[string]*documentState
	dm.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		out = make(map[string]*documentState, len(dm.docs))
		maps.Copy(out, dm.docs)
	})
	return out
}

// GetDocumentIDs returns the IDs of connected documents.
func (dm *DocumentManager) GetDocumentIDs() []string {
	var ids []string
	dm.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, doc := range dm.docs {
			if doc.connected {
				ids = append(ids, doc.id)
			}
		}
	})
	return ids
}

// GetDefaultDocID returns the default document ID.
func (dm *DocumentManager) GetDefaultDocID() string {
	var id string
	dm.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		id = dm.defaultDocID
	})
	return id
}

// WaitDefaultDoc waits for a default document to be set.
func (dm *DocumentManager) WaitDefaultDoc(ctx context.Context) (string, error) {
	for {
		var id string
		var ch <-chan struct{}
		dm.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			id = dm.defaultDocID
		})

		if id != "" {
			return id, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ch:
		}
	}
}

// HandleWebDocumentRpc handles a Go->JS RPC stream via the document manager.
func (dm *DocumentManager) HandleWebDocumentRpc(
	ctx context.Context,
	componentID string,
	_ func(),
) (srpc.Invoker, func(), error) {
	doc := dm.waitForDoc(ctx, componentID)
	if doc == nil {
		return nil, nil, errors.New("document " + componentID + " not found")
	}

	openStreamFn := func(
		ctx context.Context,
		msgHandler srpc.PacketDataHandler,
		closeHandler srpc.CloseHandler,
	) (srpc.PacketWriter, error) {
		return dm.WebDocumentOpenStream(ctx, msgHandler, closeHandler, componentID)
	}
	client := srpc.NewClient(openStreamFn)
	invoker := srpc.NewClientInvoker(client)

	return invoker, func() {}, nil
}

// WatchWebRuntimeStatus streams document status updates to the Remote.
func (dm *DocumentManager) WatchWebRuntimeStatus(_ *web_runtime.WatchWebRuntimeStatusRequest, strm web_runtime.SRPCWebRuntime_WatchWebRuntimeStatusStream) error {
	ctx := strm.Context()

	var initial bool
	for {
		var ch <-chan struct{}
		var status *web_runtime.WebRuntimeStatus
		dm.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			status = &web_runtime.WebRuntimeStatus{Snapshot: true}
			for _, doc := range dm.docs {
				if doc.connected {
					status.WebDocuments = append(status.WebDocuments, &web_runtime.WebDocumentStatus{
						Id:        doc.id,
						Permanent: true,
					})
				}
			}
		})

		if !initial {
			dm.le.Debugf("WatchWebRuntimeStatus: sending initial snapshot with %d docs", len(status.WebDocuments))
			initial = true
		}

		if err := strm.Send(status); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			dm.le.Debug("WatchWebRuntimeStatus: context canceled")
			return ctx.Err()
		case <-ch:
		}
	}
}

// WebDocumentRpc handles a Go->JS RPC stream via the SRPC protocol.
func (dm *DocumentManager) WebDocumentRpc(strm web_runtime.SRPCWebRuntime_WebDocumentRpcStream) error {
	return rpcstream.HandleRpcStream(strm, dm.HandleWebDocumentRpc)
}

// CreateWebDocument is not supported for saucer (single window).
func (dm *DocumentManager) CreateWebDocument(_ context.Context, _ *web_runtime.CreateWebDocumentRequest) (*web_runtime.CreateWebDocumentResponse, error) {
	return &web_runtime.CreateWebDocumentResponse{}, nil
}

// RemoveWebDocument is not supported for saucer.
func (dm *DocumentManager) RemoveWebDocument(_ context.Context, _ *web_runtime.RemoveWebDocumentRequest) (*web_runtime.RemoveWebDocumentResponse, error) {
	return &web_runtime.RemoveWebDocumentResponse{}, nil
}

// WebWorkerRpc is not supported for saucer.
func (dm *DocumentManager) WebWorkerRpc(_ web_runtime.SRPCWebRuntime_WebWorkerRpcStream) error {
	return errors.New("web workers not supported in saucer")
}

// _ is a type assertion
var _ rpcstream.RpcStreamGetter = ((*DocumentManager)(nil)).HandleWebDocumentRpc

// _ is a type assertion
var _ web_runtime.SRPCWebRuntimeServer = ((*DocumentManager)(nil))
