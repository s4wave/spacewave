package bifrost_http

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
)

var errHTTPHandlerResolveTimeout = errors.New("timed out waiting for HTTP handler")

const httpHandlerResolveTimeout = 30 * time.Second

// HTTPHandler implements a HTTP handler which deduplicates with a reference count.
type HTTPHandler struct {
	// handleCtr is the refcount handle to the UnixFS
	handleCtr *ccontainer.CContainer[http.Handler]
	// errCtr contains any error building FSHandle
	errCtr *ccontainer.CContainer[*error]
	// rc is the refcount container
	rc *refcount.RefCount[http.Handler]
}

// NewHTTPHandler constructs a new HTTPHandler.
//
// NOTE: if ctx == nil the handler won't work until SetContext is called.
func NewHTTPHandler(
	ctx context.Context,
	builder HTTPHandlerBuilder,
) *HTTPHandler {
	h := &HTTPHandler{
		handleCtr: ccontainer.NewCContainer[http.Handler](nil),
		errCtr:    ccontainer.NewCContainer[*error](nil),
	}
	h.rc = refcount.NewRefCount(ctx, false, h.handleCtr, h.errCtr, builder)
	return h
}

// SetContext sets the context for the HTTPHandler.
func (h *HTTPHandler) SetContext(ctx context.Context) {
	h.rc.SetContext(ctx)
}

// ServeHTTP serves a http request.
func (h *HTTPHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	for {
		var released sync.Once
		releasedCh := make(chan struct{})
		resolveCtx, resolveCancel := buildHTTPHandlerResolveContext(ctx)
		access, accessRel, err := h.rc.ResolveWithReleased(resolveCtx, func() {
			released.Do(func() {
				close(releasedCh)
			})
		})
		resolveCancel()
		if err != nil {
			if resolveCtx.Err() != nil &&
				(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				err = errors.Wrap(context.Cause(resolveCtx), errHTTPHandlerResolveTimeout.Error())
			}
			rw.WriteHeader(500)
			_, _ = rw.Write([]byte(err.Error())) //nolint:gosec // internal error, not user-controlled
			return
		}
		if access == nil {
			rw.WriteHeader(404)
			_, _ = rw.Write([]byte("404 not found"))
			accessRel()
			return
		}
		serveCtx, serveCancel := context.WithCancel(ctx)
		stateRw := newResponseStateWriter(rw)
		go func() {
			select {
			case <-ctx.Done():
			case <-serveCtx.Done():
			case <-releasedCh:
				serveCancel()
			}
		}()
		access.ServeHTTP(stateRw, req.WithContext(serveCtx))
		serveCancel()
		accessRel()
		if ctx.Err() != nil || stateRw.Committed() {
			return
		}
		select {
		case <-releasedCh:
			continue
		default:
			return
		}
	}
}

// _ is a type assertion
var _ http.Handler = ((*HTTPHandler)(nil))

type responseStateWriter struct {
	rw        http.ResponseWriter
	committed atomic.Bool
}

func newResponseStateWriter(rw http.ResponseWriter) *responseStateWriter {
	return &responseStateWriter{rw: rw}
}

func buildHTTPHandlerResolveContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeoutCause(ctx, httpHandlerResolveTimeout, errHTTPHandlerResolveTimeout)
}

func (w *responseStateWriter) Header() http.Header {
	return w.rw.Header()
}

func (w *responseStateWriter) WriteHeader(statusCode int) {
	w.committed.Store(true)
	w.rw.WriteHeader(statusCode)
}

func (w *responseStateWriter) Write(p []byte) (int, error) {
	w.committed.Store(true)
	return w.rw.Write(p)
}

func (w *responseStateWriter) Committed() bool {
	return w.committed.Load()
}

func (w *responseStateWriter) Unwrap() http.ResponseWriter {
	return w.rw
}

func (w *responseStateWriter) Flush() {
	if f, ok := w.rw.(http.Flusher); ok {
		w.committed.Store(true)
		f.Flush()
	}
}

func (w *responseStateWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.rw.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	w.committed.Store(true)
	return h.Hijack()
}

func (w *responseStateWriter) Push(target string, opts *http.PushOptions) error {
	p, ok := w.rw.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return p.Push(target, opts)
}

func (w *responseStateWriter) ReadFrom(r io.Reader) (int64, error) {
	rf, ok := w.rw.(io.ReaderFrom)
	if !ok {
		w.committed.Store(true)
		return io.Copy(w.rw, r)
	}
	w.committed.Store(true)
	return rf.ReadFrom(r)
}

// _ is a type assertion
var _ http.ResponseWriter = ((*responseStateWriter)(nil))

// _ is a type assertion
var _ http.Flusher = ((*responseStateWriter)(nil))

// _ is a type assertion
var _ http.Hijacker = ((*responseStateWriter)(nil))

// _ is a type assertion
var _ http.Pusher = ((*responseStateWriter)(nil))

// _ is a type assertion
var _ io.ReaderFrom = ((*responseStateWriter)(nil))
