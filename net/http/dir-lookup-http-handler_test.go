package bifrost_http

import (
	"net/http"
	"net/url"
	"testing"
)

func TestMatchServeMuxPattern(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("GET /test/nested", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("POST /test", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("GET /posts/{id}", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("GET /posts/latest", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/files/{pathname...}", func(w http.ResponseWriter, r *http.Request) {})

	tests := []struct {
		name           string
		method         string
		url            string
		expectedPath   string
		expectedExists bool
	}{
		{
			name:           "Exact match GET",
			method:         "GET",
			url:            "/test",
			expectedPath:   "GET /test",
			expectedExists: true,
		},
		{
			name:           "Nested match GET",
			method:         "GET",
			url:            "/test/nested",
			expectedPath:   "GET /test/nested",
			expectedExists: true,
		},
		{
			name:           "POST method",
			method:         "POST",
			url:            "/test",
			expectedPath:   "POST /test",
			expectedExists: true,
		},
		{
			name:           "Wildcard match",
			method:         "GET",
			url:            "/posts/123",
			expectedPath:   "GET /posts/{id}",
			expectedExists: true,
		},
		{
			name:           "Specific path over wildcard",
			method:         "GET",
			url:            "/posts/latest",
			expectedPath:   "GET /posts/latest",
			expectedExists: true,
		},
		{
			name:           "Wildcard with multiple segments",
			method:         "GET",
			url:            "/files/path/to/file.txt",
			expectedPath:   "/files/{pathname...}",
			expectedExists: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedURL, err := url.Parse(tt.url)
			if err != nil {
				t.Fatalf("Failed to parse URL: %v", err)
			}

			dir := NewLookupHTTPHandler(tt.method, parsedURL, "")
			handler, pattern := MatchServeMuxPattern(mux, dir)

			if tt.expectedExists {
				if handler == nil {
					t.Fatal("Expected handler to not be nil")
				}
				if pattern != tt.expectedPath {
					t.Fatalf("Expected pattern %s, got %s", tt.expectedPath, pattern)
				}
			} else {
				if handler != nil {
					t.Fatalf("Expected handler to be nil, got %v", handler)
				}
				if pattern != "" {
					t.Fatalf("Expected empty pattern, got %s", pattern)
				}
			}
		})
	}
}
