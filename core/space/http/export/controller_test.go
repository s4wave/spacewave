package space_http_export

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"testing"
)

func TestExportURLParsing(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantErr  bool
		wantIdx  uint32
		wantSoID string
		wantPath string
	}{
		{
			name:     "export all",
			path:     "/export/u/0/so/my-space",
			wantIdx:  0,
			wantSoID: "my-space",
			wantPath: "u/0/so/my-space",
		},
		{
			name:     "export projected subtree",
			path:     "/export/u/1/so/abc123/-/my-object/-/nested",
			wantIdx:  1,
			wantSoID: "abc123",
			wantPath: "u/1/so/abc123/-/my-object/-/nested",
		},
		{
			name:     "export with encoded segments",
			path:     "/export/u/2/so/space-id/-/key/with%20spaces/-/child%20node",
			wantIdx:  2,
			wantSoID: "space-id",
			wantPath: "u/2/so/space-id/-/key/with spaces/-/child node",
		},
		{
			name:     "large session index",
			path:     "/export/u/42/so/test-so",
			wantIdx:  42,
			wantSoID: "test-so",
			wantPath: "u/42/so/test-so",
		},
		{
			name:    "missing prefix",
			path:    "/other/u/0/so/id",
			wantErr: true,
		},
		{
			name:    "invalid session index",
			path:    "/export/u/abc/so/id",
			wantErr: true,
		},
		{
			name:    "missing so segment",
			path:    "/export/u/0/xx/id",
			wantErr: true,
		},
		{
			name:    "too few segments",
			path:    "/export/u/0",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := parseExportURL(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if req.sessionIdx != tt.wantIdx {
				t.Fatalf("sessionIdx: got %d, want %d", req.sessionIdx, tt.wantIdx)
			}
			if req.sharedObjectID != tt.wantSoID {
				t.Fatalf("sharedObjectID: got %q, want %q", req.sharedObjectID, tt.wantSoID)
			}
			if req.projectedPath != tt.wantPath {
				t.Fatalf("projectedPath: got %q, want %q", req.projectedPath, tt.wantPath)
			}
		})
	}
}

func TestBatchExportURLParsing(t *testing.T) {
	msg := &ExportBatchRequest{
		Paths: []string{"assets/logo.png", "docs/report.txt", "assets/logo.png"},
	}
	data, err := msg.MarshalVT()
	if err != nil {
		t.Fatalf("marshal batch request: %v", err)
	}

	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("compress batch request: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close compressor: %v", err)
	}

	req, err := parseBatchExportURL(
		"/export-batch/u/7/so/test-space/-/docs/demo/-/nested/" +
			base64.RawURLEncoding.EncodeToString(compressed.Bytes()) +
			"/bundle.zip",
	)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if req.sessionIdx != 7 {
		t.Fatalf("sessionIdx: got %d, want 7", req.sessionIdx)
	}
	if req.sharedObjectID != "test-space" {
		t.Fatalf("sharedObjectID: got %q, want %q", req.sharedObjectID, "test-space")
	}
	if req.basePath != "u/7/so/test-space/-/docs/demo/-/nested" {
		t.Fatalf("basePath: got %q", req.basePath)
	}
	if req.filename != "bundle.zip" {
		t.Fatalf("filename: got %q", req.filename)
	}
	if len(req.paths) != 2 || req.paths[0] != "assets/logo.png" || req.paths[1] != "docs/report.txt" {
		t.Fatalf("paths: got %#v", req.paths)
	}
}

func TestBatchExportURLParsingAcceptsRawPayload(t *testing.T) {
	msg := &ExportBatchRequest{
		Paths: []string{"assets/logo.png", "docs/report.txt", "assets/logo.png"},
	}
	data, err := msg.MarshalVT()
	if err != nil {
		t.Fatalf("marshal batch request: %v", err)
	}

	req, err := parseBatchExportURL(
		"/export-batch/u/7/so/test-space/-/docs/demo/-/nested/" +
			base64.RawURLEncoding.EncodeToString(data) +
			"/bundle.zip",
	)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(req.paths) != 2 || req.paths[0] != "assets/logo.png" || req.paths[1] != "docs/report.txt" {
		t.Fatalf("paths: got %#v", req.paths)
	}
}

func TestBatchExportURLParsingRejectsInvalidPayload(t *testing.T) {
	req, err := parseBatchExportURL(
		"/export-batch/u/7/so/test-space/-/docs/demo/-/nested/eJwrSCzJzM9TqgUAB6ECGw/bundle.zip",
	)
	if err == nil {
		t.Fatalf("expected parse error for invalid payload, got request %#v", req)
	}
}

func TestNormalizeBatchPathsRejectsEscapes(t *testing.T) {
	if _, err := normalizeBatchPaths([]string{"../secret"}); err == nil {
		t.Fatal("expected escape path to be rejected")
	}
}

func TestResolveProjectedExportTarget(t *testing.T) {
	tests := []struct {
		name       string
		req        exportRequest
		wantLookup string
		wantRoot   string
	}{
		{
			name: "space root exports namespace contents",
			req: exportRequest{
				sessionIdx:     7,
				sharedObjectID: "space-1",
				projectedPath:  "u/7/so/space-1",
			},
			wantLookup: "u/7/so/space-1/-",
			wantRoot:   "",
		},
		{
			name: "explicit namespace root stays rootless",
			req: exportRequest{
				sessionIdx:     7,
				sharedObjectID: "space-1",
				projectedPath:  "u/7/so/space-1/-",
			},
			wantLookup: "u/7/so/space-1/-",
			wantRoot:   "",
		},
		{
			name: "subtree export keeps selected dir name",
			req: exportRequest{
				sessionIdx:     7,
				sharedObjectID: "space-1",
				projectedPath:  "u/7/so/space-1/-/docs/demo/-/assets",
			},
			wantLookup: "u/7/so/space-1/-/docs/demo/-/assets",
			wantRoot:   "assets",
		},
		{
			name: "trailing delimiter uses parent dir name",
			req: exportRequest{
				sessionIdx:     7,
				sharedObjectID: "space-1",
				projectedPath:  "u/7/so/space-1/-/docs/demo/-",
			},
			wantLookup: "u/7/so/space-1/-/docs/demo/-",
			wantRoot:   "demo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLookup, gotRoot := resolveProjectedExportTarget(&tt.req)
			if gotLookup != tt.wantLookup {
				t.Fatalf("lookupPath: got %q, want %q", gotLookup, tt.wantLookup)
			}
			if gotRoot != tt.wantRoot {
				t.Fatalf("zipRoot: got %q, want %q", gotRoot, tt.wantRoot)
			}
		})
	}
}
