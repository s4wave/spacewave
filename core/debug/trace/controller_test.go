package debug_trace

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRewritePprofRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/debugz/pprof/goroutine?debug=2", nil)
	rewrittenReq := rewritePprofRequest(req)
	if rewrittenReq.URL.Path != "/debug/pprof/goroutine" {
		t.Fatalf("expected rewritten pprof path, got %q", rewrittenReq.URL.Path)
	}
	if rewrittenReq.URL.RawQuery != "debug=2" {
		t.Fatalf("expected query string to survive rewrite, got %q", rewrittenReq.URL.RawQuery)
	}
}

func TestServePprofIndex(t *testing.T) {
	ctrl := &Controller{}
	req := httptest.NewRequest(http.MethodGet, "/debugz/pprof/", nil)
	rec := httptest.NewRecorder()
	ctrl.ServeHTTP(rec, req)
	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from pprof index, got %d", resp.StatusCode)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "full goroutine stack dump") {
		t.Fatalf("expected pprof index body, got %q", body)
	}
}

func TestServePprofGoroutine(t *testing.T) {
	ctrl := &Controller{}
	req := httptest.NewRequest(http.MethodGet, "/debugz/pprof/goroutine?debug=1", nil)
	rec := httptest.NewRecorder()
	ctrl.ServeHTTP(rec, req)
	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from goroutine profile, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("expected text goroutine dump, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), "goroutine profile") {
		t.Fatalf("expected goroutine profile output, got %q", rec.Body.String())
	}
}
