package saucer

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	"github.com/sirupsen/logrus"
)

func TestDocumentManagerDefaultDocumentWaitsForMuxRead(t *testing.T) {
	dm := newTestDocumentManager()
	waitCtx, waitCancel := context.WithTimeout(t.Context(), time.Second)
	defer waitCancel()

	ready := make(chan struct {
		docID string
		err   error
	}, 1)
	go func() {
		docID, err := dm.WaitDefaultDoc(waitCtx)
		ready <- struct {
			docID string
			err   error
		}{docID: docID, err: err}
	}()

	readCtx, readCancel := context.WithCancel(t.Context())
	readDone := startTestMuxRead(dm, readCtx, "doc-a")
	defer func() {
		readCancel()
		waitForDone(t, readDone, "mux read")
	}()

	select {
	case result := <-ready:
		if result.err != nil {
			t.Fatalf("WaitDefaultDoc returned error after mux read connected: %v", result.err)
		}
		if result.docID != "doc-a" {
			t.Fatalf("WaitDefaultDoc returned doc id %q, want doc-a", result.docID)
		}
	case <-waitCtx.Done():
		t.Fatalf("WaitDefaultDoc did not observe mux read readiness: %v", waitCtx.Err())
	}
}

func TestDocumentManagerMuxWriteWaitsForMuxRead(t *testing.T) {
	dm := newTestDocumentManager()

	cancelCtx, cancel := context.WithCancel(t.Context())
	cancelBody := &countingReader{data: []byte{9}}
	cancelDone := make(chan struct{}, 1)
	go func() {
		req := httptest.NewRequest("POST", "/b/saucer/doc-a/mux", cancelBody).
			WithContext(cancelCtx)
		dm.ServeSaucerHTTP(httptest.NewRecorder(), req)
		cancelDone <- struct{}{}
	}()
	cancel()
	waitForDone(t, cancelDone, "pre-mux canceled POST")
	if reads := cancelBody.reads.Load(); reads != 0 {
		t.Fatalf("POST body was read %d times before mux connection", reads)
	}

	postWaiting := make(chan struct{}, 1)
	dm.muxWriteWaitHook = func(docID string) {
		if docID != "doc-a" {
			return
		}
		select {
		case postWaiting <- struct{}{}:
		default:
		}
	}

	postCtx, postCancel := context.WithCancel(t.Context())
	defer postCancel()
	body := []byte{1, 2, 3, 4}

	postDone := make(chan int, 1)
	go func() {
		req := httptest.NewRequest("POST", "/b/saucer/doc-a/mux", bytes.NewReader(body)).
			WithContext(postCtx)
		rw := httptest.NewRecorder()
		dm.ServeSaucerHTTP(rw, req)
		postDone <- rw.Code
	}()

	waitCtx, waitCancel := context.WithTimeout(t.Context(), time.Second)
	defer waitCancel()
	select {
	case <-postWaiting:
	case <-waitCtx.Done():
		t.Fatalf("POST did not reach mux readiness wait: %v", waitCtx.Err())
	}

	muxCtx, muxCancel := context.WithCancel(t.Context())
	defer muxCancel()
	mc := &muxConn{
		ctx:     muxCtx,
		cancel:  muxCancel,
		writeCh: make(chan []byte, 1),
		flushCh: make(chan []byte, 1),
	}
	dm.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		dm.docs["doc-a"] = &documentState{
			id:        "doc-a",
			connected: true,
			mc:        mc,
		}
		broadcast()
	})

	select {
	case queued := <-mc.writeCh:
		if !bytes.Equal(queued, body) {
			t.Fatalf("queued body = %v, want %v", queued, body)
		}
	case <-waitCtx.Done():
		t.Fatalf("POST body did not reach mux writer after connection: %v", waitCtx.Err())
	}

	select {
	case code := <-postDone:
		if code != 204 {
			t.Fatalf("POST status = %d, want 204", code)
		}
	case <-waitCtx.Done():
		t.Fatalf("POST did not complete after mux read connected: %v", waitCtx.Err())
	}
}

func TestDocumentManagerDefaultDocAndStatusWatchFollowMuxLifecycle(t *testing.T) {
	dm := newTestDocumentManager()
	statusStream := newTestRuntimeStatusStream(t.Context())
	watchDone := make(chan error, 1)
	go func() {
		watchDone <- dm.WatchWebRuntimeStatus(web_runtime.NewWatchWebRuntimeStatusRequest(), statusStream)
	}()

	initial := statusStream.recv(t)
	if !initial.GetSnapshot() {
		t.Fatal("initial status is not a snapshot")
	}
	if got := len(initial.GetWebDocuments()); got != 0 {
		t.Fatalf("initial status document count = %d, want 0", got)
	}

	readCtx, readCancel := context.WithCancel(t.Context())
	readDone := startTestMuxRead(dm, readCtx, "doc-a")

	waitCtx, waitCancel := context.WithTimeout(t.Context(), time.Second)
	defer waitCancel()
	docID, err := dm.WaitDefaultDoc(waitCtx)
	if err != nil {
		t.Fatalf("wait default doc: %v", err)
	}
	if docID != "doc-a" {
		t.Fatalf("default doc = %q, want doc-a", docID)
	}

	connected := statusStream.recvWithDocCount(t, 1)
	docs := connected.GetWebDocuments()
	if got := docs[0].GetId(); got != "doc-a" {
		t.Fatalf("connected status doc id = %q, want doc-a", got)
	}
	if !docs[0].GetPermanent() {
		t.Fatal("connected status doc is not marked permanent")
	}

	readCancel()
	waitForDone(t, readDone, "mux read")

	statusStream.recvWithDocCount(t, 0)

	statusStream.cancel()
	select {
	case err := <-watchDone:
		if err == nil {
			t.Fatal("status watch returned nil after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("status watch did not exit after cancellation")
	}
}

func newTestDocumentManager() *DocumentManager {
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	return NewDocumentManager(logrus.NewEntry(log))
}

func startTestMuxRead(dm *DocumentManager, ctx context.Context, docID string) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest("GET", "/b/saucer/"+docID+"/mux", nil).
			WithContext(ctx)
		dm.ServeSaucerHTTP(httptest.NewRecorder(), req)
	}()
	return done
}

func waitForDone(t *testing.T, done <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("%s did not finish", name)
	}
}

type testRuntimeStatusStream struct {
	ctx      context.Context
	cancelFn context.CancelFunc
	statuses chan *web_runtime.WebRuntimeStatus
}

func newTestRuntimeStatusStream(ctx context.Context) *testRuntimeStatusStream {
	streamCtx, cancel := context.WithCancel(ctx)
	return &testRuntimeStatusStream{
		ctx:      streamCtx,
		cancelFn: cancel,
		statuses: make(chan *web_runtime.WebRuntimeStatus, 8),
	}
}

func (s *testRuntimeStatusStream) recv(t *testing.T) *web_runtime.WebRuntimeStatus {
	t.Helper()
	select {
	case status := <-s.statuses:
		return status
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runtime status")
		return nil
	}
}

func (s *testRuntimeStatusStream) recvWithDocCount(t *testing.T, docCount int) *web_runtime.WebRuntimeStatus {
	t.Helper()
	deadline := time.After(time.Second)
	var last *web_runtime.WebRuntimeStatus
	for {
		select {
		case status := <-s.statuses:
			last = status
			if len(status.GetWebDocuments()) == docCount {
				return status
			}
		case <-deadline:
			if last == nil {
				t.Fatalf("timed out waiting for runtime status with %d docs; no status received", docCount)
			}
			t.Fatalf(
				"timed out waiting for runtime status with %d docs; last had %d docs",
				docCount,
				len(last.GetWebDocuments()),
			)
		}
	}
}

func (s *testRuntimeStatusStream) cancel() {
	s.cancelFn()
}

func (s *testRuntimeStatusStream) Context() context.Context {
	return s.ctx
}

func (s *testRuntimeStatusStream) MsgSend(_ srpc.Message) error {
	return nil
}

func (s *testRuntimeStatusStream) MsgRecv(_ srpc.Message) error {
	return context.Canceled
}

func (s *testRuntimeStatusStream) CloseSend() error {
	return nil
}

func (s *testRuntimeStatusStream) Close() error {
	s.cancel()
	return nil
}

func (s *testRuntimeStatusStream) Send(status *web_runtime.WebRuntimeStatus) error {
	select {
	case s.statuses <- status:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *testRuntimeStatusStream) SendAndClose(status *web_runtime.WebRuntimeStatus) error {
	if status != nil {
		if err := s.Send(status); err != nil {
			return err
		}
	}
	return s.CloseSend()
}

var _ web_runtime.SRPCWebRuntime_WatchWebRuntimeStatusStream = (*testRuntimeStatusStream)(nil)

type countingReader struct {
	data  []byte
	reads atomic.Int32
}

func (r *countingReader) Read(p []byte) (int, error) {
	r.reads.Add(1)
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
