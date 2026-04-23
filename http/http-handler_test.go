package bifrost_http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/bifrost/testbed"
	"github.com/sirupsen/logrus"
)

func TestHTTPHandler(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	tb, err := testbed.NewTestbed(ctx, le, testbed.TestbedOpts{
		NoEcho: true,
		NoPeer: true,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer startMockHandler(t, tb)()

	busHandler := NewBusHandler(tb.Bus, "test-client", true)
	handler := NewHTTPHandler(ctx, NewHTTPHandlerBuilder(busHandler))

	// perform a request
	checkMockRequest(t, handler)
}

func TestHTTPHandlerRebindsBeforeCommit(t *testing.T) {
	ctx := context.Background()
	var resolveCount atomic.Int32
	var replacementServed atomic.Int32
	handler := NewHTTPHandler(ctx, func(ctx context.Context, released func()) (http.Handler, func(), error) {
		switch resolveCount.Add(1) {
		case 1:
			return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				released()
				<-req.Context().Done()
			}), func() {}, nil
		default:
			return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				replacementServed.Add(1)
				rw.WriteHeader(200)
				_, _ = rw.Write([]byte("replacement"))
			}), func() {}, nil
		}
	})

	req := httptest.NewRequest("GET", "/foo/bar", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 status but got %v: %s", resp.StatusCode, resp.Status)
	}
	body := w.Body.String()
	if body != "replacement" {
		t.Fatalf("expected replacement body, got %q", body)
	}
	if replacementServed.Load() != 1 {
		t.Fatalf("expected replacement handler to serve once, got %d", replacementServed.Load())
	}
}

func TestHTTPHandlerDoesNotReplayAfterCommit(t *testing.T) {
	ctx := context.Background()
	var resolveCount atomic.Int32
	var replacementServed atomic.Int32
	handler := NewHTTPHandler(ctx, func(ctx context.Context, released func()) (http.Handler, func(), error) {
		switch resolveCount.Add(1) {
		case 1:
			return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				rw.WriteHeader(200)
				_, _ = rw.Write([]byte("first"))
				released()
			}), func() {}, nil
		default:
			return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				replacementServed.Add(1)
				rw.WriteHeader(200)
				_, _ = rw.Write([]byte("replacement"))
			}), func() {}, nil
		}
	})

	req := httptest.NewRequest("GET", "/foo/bar", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 status but got %v: %s", resp.StatusCode, resp.Status)
	}
	body := w.Body.String()
	if body != "first" {
		t.Fatalf("expected first body, got %q", body)
	}
	if replacementServed.Load() != 0 {
		t.Fatalf("expected replacement handler to not serve, got %d", replacementServed.Load())
	}
}

func TestHTTPHandlerTimesOutWaitingForReplacement(t *testing.T) {
	ctx := context.Background()
	var resolveCount atomic.Int32
	handler := NewHTTPHandler(ctx, func(ctx context.Context, released func()) (http.Handler, func(), error) {
		switch resolveCount.Add(1) {
		case 1:
			return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				released()
				<-req.Context().Done()
			}), func() {}, nil
		default:
			<-ctx.Done()
			return nil, nil, ctx.Err()
		}
	})

	reqCtx, reqCancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer reqCancel()
	req := httptest.NewRequest("GET", "/foo/bar", nil).WithContext(reqCtx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != 500 {
		t.Fatalf("expected 500 status but got %v: %s", resp.StatusCode, resp.Status)
	}
	body := w.Body.String()
	if !strings.Contains(body, errHTTPHandlerResolveTimeout.Error()) {
		t.Fatalf("expected timeout error body, got %q", body)
	}
}
