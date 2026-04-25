package space_http_header

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetAttachmentHeaderASCII(t *testing.T) {
	w := httptest.NewRecorder()
	SetAttachmentHeader(w, "hello.txt")
	if got := w.Header().Get("Content-Disposition"); got != `attachment; filename=hello.txt` {
		t.Fatalf("unexpected content disposition: %q", got)
	}
}

func TestSetAttachmentHeaderUnicode(t *testing.T) {
	w := httptest.NewRecorder()
	SetAttachmentHeader(w, "Screenshot 2026-04-07 at 11.29.44\u202fPM.png")
	got := w.Header().Get("Content-Disposition")
	if strings.ContainsRune(got, '\u202f') {
		t.Fatalf("content disposition contains raw unicode: %q", got)
	}
	if !strings.Contains(got, "filename*=") {
		t.Fatalf("content disposition missing filename*: %q", got)
	}
}
