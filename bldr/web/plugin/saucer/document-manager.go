package saucer

import (
	"context"
	"encoding/binary"
	"io"
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
	id         string
	generation uint64
	connected  bool

	// mux is the yamux mux connection to JS.
	// Set when JS connects via /b/saucer/{docId}/mux GET.
	mux srpc.MuxedConn

	// mc is the underlying muxConn for posting data from JS.
	mc *muxConn
}

type documentSessionOwner struct {
	// bcast guards docs and defaultDocID and broadcasts on changes.
	bcast broadcast.Broadcast
	docs  map[string]*documentState

	// defaultDocID is the document ID for incoming Go->JS streams.
	defaultDocID string

	// snapshotCtr contains the current WebRuntimeStatus snapshot.
	snapshotCtr *ccontainer.CContainer[*web_runtime.WebRuntimeStatus]

	// muxWriteWaitHook is test-only synchronization for pre-GET POST ordering.
	// Production leaves it nil; if set, it must be nonblocking.
	muxWriteWaitHook func(docID string)
}

type documentSession struct {
	docID      string
	generation uint64
}

// DocumentManager tracks connected web documents and manages RPC streams
// via yamux multiplexed over a single HTTP streaming connection per document.
type DocumentManager struct {
	le       *logrus.Entry
	sessions documentSessionOwner

	// server is the SRPC server for handling JS-initiated RPC streams.
	server *srpc.Server
}

// NewDocumentManager constructs a new DocumentManager.
func NewDocumentManager(le *logrus.Entry) *DocumentManager {
	return &DocumentManager{
		le: le,
		sessions: documentSessionOwner{
			docs:        make(map[string]*documentState),
			snapshotCtr: ccontainer.NewCContainerVT[*web_runtime.WebRuntimeStatus](nil),
		},
	}
}

// SetServer sets the SRPC server for handling JS-initiated RPC streams.
func (dm *DocumentManager) SetServer(srv *srpc.Server) {
	dm.server = srv
}

// GetWebRuntimeStatusCtr returns the status container.
func (dm *DocumentManager) GetWebRuntimeStatusCtr() *ccontainer.CContainer[*web_runtime.WebRuntimeStatus] {
	return dm.sessions.snapshotCtr
}

func (o *documentSessionOwner) beginMuxReconnect(docID string) (uint64, srpc.MuxedConn) {
	var generation uint64
	var oldMux srpc.MuxedConn
	var status *web_runtime.WebRuntimeStatus
	o.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		doc := o.getOrCreateDocLocked(docID)
		oldMux = doc.mux
		doc.generation++
		generation = doc.generation
		doc.mux = nil
		doc.mc = nil
		doc.connected = false
		status = o.statusLocked()
		broadcast()
	})
	o.publishStatus(status)
	return generation, oldMux
}

func (o *documentSessionOwner) connectMux(docID string, generation uint64, mux srpc.MuxedConn, mc *muxConn) bool {
	var status *web_runtime.WebRuntimeStatus
	var connected bool
	o.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		doc := o.getOrCreateDocLocked(docID)
		if doc.generation != generation {
			return
		}
		doc.mux = mux
		doc.mc = mc
		doc.connected = true
		o.defaultDocID = docID
		status = o.statusLocked()
		connected = true
		broadcast()
	})
	o.publishStatus(status)
	return connected
}

func (o *documentSessionOwner) disconnectMux(session documentSession) {
	var status *web_runtime.WebRuntimeStatus
	var closeMux srpc.MuxedConn
	o.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		doc := o.docs[session.docID]
		if doc == nil || doc.generation != session.generation {
			return
		}
		closeMux = doc.mux
		doc.mux = nil
		doc.mc = nil
		doc.connected = false
		status = o.statusLocked()
		broadcast()
	})
	if closeMux != nil {
		_ = closeMux.Close()
	}
	o.publishStatus(status)
}

func (o *documentSessionOwner) close() []srpc.MuxedConn {
	var muxes []srpc.MuxedConn
	var status *web_runtime.WebRuntimeStatus
	o.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		for _, doc := range o.docs {
			if doc.mux != nil {
				muxes = append(muxes, doc.mux)
			}
		}
		o.docs = make(map[string]*documentState)
		status = o.statusLocked()
		broadcast()
	})
	o.publishStatus(status)
	return muxes
}

func (o *documentSessionOwner) waitMux(ctx context.Context, docID string) (srpc.MuxedConn, error) {
	for {
		var mux srpc.MuxedConn
		var ch <-chan struct{}
		o.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			if doc := o.docs[docID]; doc != nil && doc.connected {
				mux = doc.mux
			}
		})
		if mux != nil {
			return mux, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ch:
		}
	}
}

func (o *documentSessionOwner) waitMuxConn(ctx context.Context, docID string) (*muxConn, error) {
	for {
		var mc *muxConn
		var ch <-chan struct{}
		var waitHook func(string)
		o.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			waitHook = o.muxWriteWaitHook
			if doc := o.docs[docID]; doc != nil && doc.connected {
				mc = doc.mc
			}
		})
		if mc != nil {
			return mc, nil
		}
		if waitHook != nil {
			waitHook(docID)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ch:
		}
	}
}

func (o *documentSessionOwner) webDocuments() map[string]*documentState {
	var out map[string]*documentState
	o.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		out = make(map[string]*documentState, len(o.docs))
		for id, doc := range o.docs {
			copied := *doc
			out[id] = &copied
		}
	})
	return out
}

func (o *documentSessionOwner) documentIDs() []string {
	var ids []string
	o.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, doc := range o.docs {
			if doc.connected {
				ids = append(ids, doc.id)
			}
		}
	})
	return ids
}

func (o *documentSessionOwner) defaultDocIDValue() string {
	var id string
	o.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		id = o.defaultDocID
	})
	return id
}

func (o *documentSessionOwner) waitDefaultDoc(ctx context.Context) (string, error) {
	for {
		var id string
		var ch <-chan struct{}
		o.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			id = o.defaultDocID
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

func (o *documentSessionOwner) watchStatus(
	ctx context.Context,
	send func(*web_runtime.WebRuntimeStatus) error,
) error {
	for {
		var ch <-chan struct{}
		var status *web_runtime.WebRuntimeStatus
		o.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			status = o.statusLocked()
		})

		if err := send(status); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

func (o *documentSessionOwner) getOrCreateDocLocked(id string) *documentState {
	doc, ok := o.docs[id]
	if !ok {
		doc = &documentState{id: id}
		o.docs[id] = doc
	}
	return doc
}

func (o *documentSessionOwner) statusLocked() *web_runtime.WebRuntimeStatus {
	status := &web_runtime.WebRuntimeStatus{Snapshot: true}
	for _, doc := range o.docs {
		if doc.connected {
			status.WebDocuments = append(status.WebDocuments, &web_runtime.WebDocumentStatus{
				Id:        doc.id,
				Permanent: true,
			})
		}
	}
	return status
}

func (o *documentSessionOwner) publishStatus(status *web_runtime.WebRuntimeStatus) {
	if status != nil {
		o.snapshotCtr.SetValue(status)
	}
}

// Close closes all documents.
func (dm *DocumentManager) Close() {
	for _, mux := range dm.sessions.close() {
		_ = mux.Close()
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
	generation, oldMux := dm.sessions.beginMuxReconnect(docID)
	if oldMux != nil {
		_ = oldMux.Close()
	}

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
	if !dm.sessions.connectMux(docID, generation, yamuxConn, mc) {
		muxCancel()
		_ = yamuxConn.Close()
		rw.WriteHeader(409)
		_, _ = rw.Write([]byte("stale saucer mux"))
		return
	}
	session := documentSession{
		docID:      docID,
		generation: generation,
	}
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
			dm.sessions.disconnectMux(session)
			return
		case <-req.Context().Done():
			muxCancel()
			dm.sessions.disconnectMux(session)
			return
		case data, ok := <-mc.flushCh:
			if !ok {
				dm.sessions.disconnectMux(session)
				return
			}
			if _, err := rw.Write(data); err != nil {
				dm.le.WithField("doc-id", docID).WithError(err).Debug("mux read write error")
				muxCancel()
				dm.sessions.disconnectMux(session)
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
	mc, err := dm.sessions.waitMuxConn(req.Context(), docID)
	if err != nil {
		return
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
	mux, err := dm.sessions.waitMux(ctx, webDocumentID)
	if err != nil {
		return nil, err
	}
	stream, err := mux.OpenStream(ctx)
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
	return dm.sessions.webDocuments()
}

// GetDocumentIDs returns the IDs of connected documents.
func (dm *DocumentManager) GetDocumentIDs() []string {
	return dm.sessions.documentIDs()
}

// GetDefaultDocID returns the default document ID.
func (dm *DocumentManager) GetDefaultDocID() string {
	return dm.sessions.defaultDocIDValue()
}

// WaitDefaultDoc waits for a default document to be set.
func (dm *DocumentManager) WaitDefaultDoc(ctx context.Context) (string, error) {
	return dm.sessions.waitDefaultDoc(ctx)
}

// HandleWebDocumentRpc handles a Go->JS RPC stream via the document manager.
func (dm *DocumentManager) HandleWebDocumentRpc(
	ctx context.Context,
	componentID string,
	_ func(),
) (srpc.Invoker, func(), error) {
	if _, err := dm.sessions.waitMux(ctx, componentID); err != nil {
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
	err := dm.sessions.watchStatus(ctx, func(status *web_runtime.WebRuntimeStatus) error {
		if !initial {
			dm.le.Debugf("WatchWebRuntimeStatus: sending initial snapshot with %d docs", len(status.WebDocuments))
			initial = true
		}
		return strm.Send(status)
	})
	if ctx.Err() != nil {
		dm.le.Debug("WatchWebRuntimeStatus: context canceled")
	}
	return err
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

// FlushIndexCache is not supported for saucer.
func (dm *DocumentManager) FlushIndexCache(_ context.Context, _ *web_runtime.FlushIndexCacheRequest) (*web_runtime.FlushIndexCacheResponse, error) {
	return nil, errors.New("index cache refresh not supported in saucer")
}

// WebWorkerRpc is not supported for saucer.
func (dm *DocumentManager) WebWorkerRpc(_ web_runtime.SRPCWebRuntime_WebWorkerRpcStream) error {
	return errors.New("web workers not supported in saucer")
}

// _ is a type assertion
var _ rpcstream.RpcStreamGetter = ((*DocumentManager)(nil)).HandleWebDocumentRpc

// _ is a type assertion
var _ web_runtime.SRPCWebRuntimeServer = ((*DocumentManager)(nil))
