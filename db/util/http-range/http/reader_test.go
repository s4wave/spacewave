package http_range_http

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// TestReadAtFillsFromShortRanges checks that ReadAt fills the buffer when the
// server answers each range request with fewer bytes than requested.
func TestReadAtFillsFromShortRanges(t *testing.T) {
	// Serve content in ranges of at most three bytes.
	content := []byte("0123456789abcdef")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Parse the first byte of the requested range.
		from, _, _ := strings.Cut(strings.TrimPrefix(req.Header.Get("Range"), "bytes="), "-")
		start, err := strconv.Atoi(from)
		if err != nil || start >= len(content) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}

		// Answer with at most three bytes from that offset.
		end := min(start+3, len(content))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[start:end])
	}))
	defer srv.Close()

	// Read ten bytes from offset two through the range reader.
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	rd := NewHTTPRangeReader(nil, req, srv.Client(), false)
	buf := make([]byte, 10)
	n, err := rd.ReadAt(buf, 2)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(buf) || !bytes.Equal(buf, content[2:12]) {
		t.Fatalf("read %d bytes %q, want %q", n, buf[:n], content[2:12])
	}

	// Read past the end with a known size and expect a short read with io.EOF.
	rd.SetSize(uint64(len(content)))
	n, err = rd.ReadAt(buf, 12)
	if err != io.EOF || n != 4 || !bytes.Equal(buf[:n], content[12:]) {
		t.Fatalf("read %d bytes %q with %v, want %q with io.EOF", n, buf[:n], err, content[12:])
	}
}
