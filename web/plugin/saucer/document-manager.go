package saucer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/bldr/util/framedstream"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/sirupsen/logrus"
)

// controlMessage is a JSON message sent on the control stream to JS.
type controlMessage struct {
	Type string `json:"type"`
	ID   int32  `json:"id,omitempty"`
}

// streamState tracks a single RPC stream between JS and Go.
type streamState struct {
	// serverOnce ensures the SRPC server handler is started exactly once
	// for JS-initiated streams.
	serverOnce sync.Once

	// toJS is data going to JS.
	toJS chan []byte
	// fromJS is data coming from JS.
	fromJS chan []byte

	// goInitiated indicates this stream was initiated by Go.
	goInitiated bool
	// closed indicates the stream is closed.
	closed atomic.Bool
	// closeCh is closed when the stream is closed.
	closeCh chan struct{}
}

// Close closes the stream state.
func (s *streamState) Close() {
	if s.closed.Swap(true) {
		return
	}
	close(s.closeCh)
}

// documentState tracks all streams for a single web document.
type documentState struct {
	id        string
	connected atomic.Bool

	mtx     sync.Mutex
	streams map[int32]*streamState

	// controlCh receives messages for the control stream.
	controlCh chan controlMessage

	// nextIncomingID is the next stream ID for Go-initiated streams (negative).
	nextIncomingID atomic.Int32
}

// newDocumentState constructs a new documentState.
func newDocumentState(id string) *documentState {
	d := &documentState{
		id:        id,
		streams:   make(map[int32]*streamState),
		controlCh: make(chan controlMessage, 64),
	}
	d.nextIncomingID.Store(-1)
	return d
}

// getOrCreateStream returns or creates a stream.
func (d *documentState) getOrCreateStream(id int32) *streamState {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	s, ok := d.streams[id]
	if !ok {
		s = &streamState{
			toJS:    make(chan []byte, 64),
			fromJS:  make(chan []byte, 64),
			closeCh: make(chan struct{}),
		}
		d.streams[id] = s
	}
	return s
}

// closeAll closes all streams and the control channel.
func (d *documentState) closeAll() {
	d.mtx.Lock()
	streams := d.streams
	d.streams = make(map[int32]*streamState)
	close(d.controlCh)
	d.controlCh = make(chan controlMessage, 64)
	d.mtx.Unlock()
	for _, s := range streams {
		s.Close()
	}
	d.connected.Store(false)
}

// DocumentManager tracks connected web documents and manages RPC streams.
// Replaces the C++ ConnectionManager.
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
	// Set via SetServer after Remote creation.
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
			d = newDocumentState(id)
			dm.docs[id] = d
		}
	})
	return d
}

// getDoc returns an existing document state.
func (dm *DocumentManager) getDoc(id string) *documentState {
	var d *documentState
	dm.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		d = dm.docs[id]
	})
	return d
}

// updateStatusSnapshot updates the WebRuntimeStatus snapshot.
func (dm *DocumentManager) updateStatusSnapshot() {
	var status *web_runtime.WebRuntimeStatus
	dm.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		status = &web_runtime.WebRuntimeStatus{Snapshot: true}
		for _, doc := range dm.docs {
			if doc.connected.Load() {
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
		d.closeAll()
	}
}

// ServeSaucerHTTP handles /b/saucer/* routes.
func (dm *DocumentManager) ServeSaucerHTTP(rw http.ResponseWriter, req *http.Request) {
	path := req.URL.Path

	// Parse /b/saucer/{docId}/{remainder}
	docID, remainder, ok := parseSaucerPath(path)
	if !ok {
		rw.WriteHeader(404)
		_, _ = rw.Write([]byte("invalid saucer path"))
		return
	}

	switch remainder {
	case "connect":
		dm.handleConnect(rw, docID)
	case "control":
		dm.handleControl(rw, req, docID)
	default:
		// Check for stream routes: stream/{streamId}/{operation}
		streamID, operation, ok := parseStreamPath(remainder)
		if !ok {
			rw.WriteHeader(404)
			_, _ = rw.Write([]byte("unknown saucer route"))
			return
		}

		switch operation {
		case "read":
			dm.handleStreamRead(rw, req, docID, streamID)
		case "write":
			dm.handleStreamWrite(rw, req, docID, streamID)
		default:
			rw.WriteHeader(404)
			_, _ = rw.Write([]byte("unknown stream operation"))
		}
	}
}

// handleConnect handles /b/saucer/{docId}/connect.
func (dm *DocumentManager) handleConnect(rw http.ResponseWriter, docID string) {
	doc := dm.getOrCreateDoc(docID)

	// If already connected, close old streams (page reload).
	if doc.connected.Load() {
		doc.closeAll()
	}

	doc.connected.Store(true)

	dm.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		dm.defaultDocID = docID
		broadcast()
	})

	dm.updateStatusSnapshot()
	dm.le.WithField("doc-id", docID).Debug("document connected")

	rw.WriteHeader(200)
	_, _ = rw.Write([]byte("OK"))
}

// handleControl handles GET /b/saucer/{docId}/control.
func (dm *DocumentManager) handleControl(rw http.ResponseWriter, req *http.Request, docID string) {
	doc := dm.getDoc(docID)
	if doc == nil {
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte("document not found"))
		return
	}

	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Content-Type", "application/x-ndjson")
	rw.WriteHeader(200)

	ctx := req.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-doc.controlCh:
			if !ok {
				return
			}
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			data = append(data, '\n')
			_, err = rw.Write(data)
			if err != nil {
				return
			}
		}
	}
}

// handleStreamRead handles GET /b/saucer/{docId}/stream/{id}/read.
func (dm *DocumentManager) handleStreamRead(rw http.ResponseWriter, req *http.Request, docID string, streamID int32) {
	doc := dm.getDoc(docID)
	if doc == nil {
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte("document not found"))
		return
	}

	ss := doc.getOrCreateStream(streamID)

	// For JS-initiated streams, start the local SRPC server handler.
	// The read request is long-lived, so its context controls the stream lifetime.
	if !ss.goInitiated {
		ss.serverOnce.Do(func() {
			if dm.server == nil {
				dm.le.Error("SRPC server not set, cannot handle JS-initiated stream")
				return
			}
			bridge := &streamBridge{ss: ss, ctx: req.Context()}
			go dm.server.HandleStream(req.Context(), bridge)
		})
	}

	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Content-Type", "application/octet-stream")
	rw.WriteHeader(200)

	ctx := req.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ss.toJS:
			if !ok || ss.closed.Load() {
				return
			}
			_, err := rw.Write(data)
			if err != nil {
				return
			}
		}
	}
}

// handleStreamWrite handles POST /b/saucer/{docId}/stream/{id}/write.
func (dm *DocumentManager) handleStreamWrite(rw http.ResponseWriter, req *http.Request, docID string, streamID int32) {
	doc := dm.getDoc(docID)
	if doc == nil {
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte("document not found"))
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte("failed to read body"))
		return
	}

	ss := doc.getOrCreateStream(streamID)

	// If the stream is already closed (e.g. server finished a unary RPC before
	// the client sent its final completion packet), silently discard the data.
	if ss.closed.Load() {
		rw.WriteHeader(204)
		return
	}

	// Route data to fromJS channel.
	// For Go-initiated streams, directReadPump reads this.
	// For JS-initiated streams, Server.HandleStream reads this.
	select {
	case ss.fromJS <- body:
	case <-ss.closeCh:
		// Stream closed while we were waiting to send - discard data.
		rw.WriteHeader(204)
		return
	case <-req.Context().Done():
		return
	}

	rw.WriteHeader(204)
}

// WebDocumentOpenStream opens an RPC stream with the given WebDocument.
// This is called by Go when it wants to talk to JS.
func (dm *DocumentManager) WebDocumentOpenStream(
	ctx context.Context,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
	webDocumentID string,
) (srpc.PacketWriter, error) {
	// Wait for the document to be connected.
	doc := dm.waitForDoc(ctx, webDocumentID)
	if doc == nil {
		return nil, ctx.Err()
	}

	// Allocate a negative stream ID for Go-initiated stream.
	streamID := doc.nextIncomingID.Add(-1) + 1
	ss := doc.getOrCreateStream(streamID)
	ss.goInitiated = true

	// Notify JS about the new stream.
	select {
	case doc.controlCh <- controlMessage{Type: "stream", ID: streamID}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Create a bridge that converts fromJS/toJS channels to an io.ReadWriteCloser.
	bridge := &streamBridge{ss: ss, ctx: ctx}
	stream := framedstream.New(ctx, bridge)

	// Use direct SRPC packet framing (no RpcStreamPacket wrapper).
	// JS sends/receives raw SRPC packets with length-prefix framing,
	// not wrapped in RpcStreamPacket protobuf messages.
	go directReadPump(stream, msgHandler, closeHandler)

	return &directPacketWriter{stream: stream}, nil
}

// directPacketWriter writes SRPC packets directly with length-prefix framing.
// Bypasses the RpcStreamPacket layer that JS does not use.
type directPacketWriter struct {
	stream *framedstream.Stream
}

// WritePacket writes a packet to the remote.
func (w *directPacketWriter) WritePacket(p *srpc.Packet) error {
	data, err := p.MarshalVT()
	if err != nil {
		return err
	}
	return w.stream.SendRaw(data)
}

// Close signals that the writer will no longer send data.
// Does not close the underlying stream - the read pump may still
// be receiving responses from JS.
func (w *directPacketWriter) Close() error {
	return nil
}

// Context returns the stream context.
func (w *directPacketWriter) Context() context.Context {
	return w.stream.Context()
}

// directReadPump reads length-prefixed SRPC packets from the stream.
// Bypasses the RpcStreamPacket layer that JS does not use.
func directReadPump(stream *framedstream.Stream, cb srpc.PacketDataHandler, closed srpc.CloseHandler) {
	var err error
	for {
		var data []byte
		data, err = stream.RecvRaw()
		if err != nil {
			break
		}
		if len(data) == 0 {
			continue
		}
		if err = cb(data); err != nil {
			break
		}
	}
	if closed != nil {
		closed(err)
	}
}

// _ is a type assertion
var _ srpc.PacketWriter = ((*directPacketWriter)(nil))

// waitForDoc waits for a document to exist and be connected.
func (dm *DocumentManager) waitForDoc(ctx context.Context, docID string) *documentState {
	for {
		var doc *documentState
		var ch <-chan struct{}
		dm.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			doc = dm.docs[docID]
		})

		if doc != nil && doc.connected.Load() {
			return doc
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ch:
		}
	}
}

// streamBridge bridges a streamState to io.ReadWriteCloser for framedstream.
type streamBridge struct {
	ss      *streamState
	ctx     context.Context
	pending []byte
}

// Read reads data from JS (fromJS channel).
func (b *streamBridge) Read(p []byte) (int, error) {
	// Return buffered data from a previous oversized read first.
	if len(b.pending) > 0 {
		n := copy(p, b.pending)
		b.pending = b.pending[n:]
		if len(b.pending) == 0 {
			b.pending = nil
		}
		return n, nil
	}

	select {
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	case data, ok := <-b.ss.fromJS:
		if !ok || b.ss.closed.Load() {
			return 0, io.EOF
		}
		n := copy(p, data)
		if n < len(data) {
			b.pending = data[n:]
		}
		return n, nil
	}
}

// Write writes data to JS (toJS channel).
func (b *streamBridge) Write(p []byte) (int, error) {
	if b.ss.closed.Load() {
		return 0, io.EOF
	}
	data := make([]byte, len(p))
	copy(data, p)
	select {
	case b.ss.toJS <- data:
		return len(p), nil
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
}

// Close closes the bridge.
func (b *streamBridge) Close() error {
	b.ss.Close()
	return nil
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

// parseStreamPath parses stream/{streamId}/{operation}.
func parseStreamPath(remainder string) (streamID int32, operation string, ok bool) {
	prefix := "stream/"
	if !strings.HasPrefix(remainder, prefix) {
		return 0, "", false
	}
	rest := remainder[len(prefix):]
	before, after, ok0 := strings.Cut(rest, "/")
	if !ok0 {
		return 0, "", false
	}
	idStr := before
	operation = after
	id, err := strconv.ParseInt(idStr, 10, 32)
	if err != nil {
		return 0, "", false
	}
	if operation != "read" && operation != "write" {
		return 0, "", false
	}
	return int32(id), operation, true
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
			if doc.connected.Load() {
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
// Bridges the RpcStream from Go to JS via the control stream notification mechanism.
func (dm *DocumentManager) HandleWebDocumentRpc(
	ctx context.Context,
	componentID string,
	_ func(),
) (srpc.Invoker, func(), error) {
	// Wait for the document to exist.
	doc := dm.waitForDoc(ctx, componentID)
	if doc == nil {
		return nil, nil, fmt.Errorf("document %s not found", componentID)
	}

	// Return an invoker that opens streams via the document manager.
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
		// Build snapshot and get wait channel atomically to avoid missed wakeups.
		var ch <-chan struct{}
		var status *web_runtime.WebRuntimeStatus
		dm.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			status = &web_runtime.WebRuntimeStatus{Snapshot: true}
			for _, doc := range dm.docs {
				if doc.connected.Load() {
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
	return fmt.Errorf("web workers not supported in saucer")
}

// _ is a type assertion
var _ rpcstream.RpcStreamGetter = ((*DocumentManager)(nil)).HandleWebDocumentRpc

// _ is a type assertion
var _ web_runtime.SRPCWebRuntimeServer = ((*DocumentManager)(nil))
