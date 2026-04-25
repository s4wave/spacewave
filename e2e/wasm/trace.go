//go:build !js

package wasm

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

// traceServiceID is the plugin-prefixed service ID for the trace service
// running inside the spacewave-core plugin worker.
const traceServiceID = "plugin/spacewave-core/" + s4wave_trace.SRPCTraceServiceServiceID

// StartTrace starts runtime trace capture in the browser plugin process.
func (s *TestSession) StartTrace(ctx context.Context, label string) error {
	if s.browserClient == nil {
		return errors.New("resources not connected")
	}
	client := s4wave_trace.NewSRPCTraceServiceClientWithServiceID(s.browserClient, traceServiceID)
	_, err := client.StartTrace(ctx, &s4wave_trace.StartTraceRequest{Label: label})
	return err
}

// StopTrace stops runtime trace capture and returns the raw trace bytes.
func (s *TestSession) StopTrace(ctx context.Context) ([]byte, error) {
	if s.browserClient == nil {
		return nil, errors.New("resources not connected")
	}
	client := s4wave_trace.NewSRPCTraceServiceClientWithServiceID(s.browserClient, traceServiceID)
	stream, err := client.StopTrace(ctx, &s4wave_trace.StopTraceRequest{})
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		buf.Write(resp.GetData())
	}
	return buf.Bytes(), nil
}

// CaptureTrace starts a trace, runs fn, stops the trace, and returns the
// raw bytes. This brackets only the profiled interaction.
func (s *TestSession) CaptureTrace(ctx context.Context, label string, fn func(ctx context.Context) error) ([]byte, error) {
	if err := s.StartTrace(ctx, label); err != nil {
		return nil, errors.Wrap(err, "start trace")
	}
	if err := fn(ctx); err != nil {
		s.StopTrace(ctx) //nolint:errcheck
		return nil, errors.Wrap(err, "profiled interaction")
	}
	data, err := s.StopTrace(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "stop trace")
	}
	return data, nil
}

// WriteTraceArtifact writes trace bytes to the given path, creating parent
// directories as needed.
func WriteTraceArtifact(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// TraceArtifactPath returns a deterministic artifact path beside the test
// file. The path is derived from the test name and a suffix:
//
//	<test-package-dir>/testdata/<TestName>.trace
func TraceArtifactPath(t testing.TB) string {
	name := sanitizeTestName(t.Name())
	return filepath.Join("testdata", name+".trace")
}

// sanitizeTestName replaces characters that are unsafe for filenames.
func sanitizeTestName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}
