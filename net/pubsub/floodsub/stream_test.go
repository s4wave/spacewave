package floodsub

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/pkg/errors"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// errStream is an io.ReadWriteCloser whose reads always fail with err.
type errStream struct{ err error }

// Read returns the configured error.
func (e *errStream) Read([]byte) (int, error) { return 0, e.err }

// Write discards the written bytes.
func (e *errStream) Write(p []byte) (int, error) { return len(p), nil }

// Close is a no-op.
func (e *errStream) Close() error { return nil }

// newTestStreamHandler builds a streamHandler logging into buf.
func newTestStreamHandler(readErr error, buf *bytes.Buffer) *streamHandler {
	logger := logrus.New()
	logger.SetOutput(buf)
	logger.SetLevel(logrus.DebugLevel)
	return &streamHandler{
		stream:    stream_packet.NewSession(&errStream{err: readErr}, 1024),
		le:        logrus.NewEntry(logger),
		ctx:       context.Background(),
		ctxCancel: func() {},
	}
}

// TestReadPumpCleanClose tests that a clean close exits at debug level.
func TestReadPumpCleanClose(t *testing.T) {
	var buf bytes.Buffer
	sh := newTestStreamHandler(errors.New("NO_ERROR"), &buf)
	sh.readPump(sh.ctx)
	if strings.Contains(buf.String(), "error receiving message") {
		t.Fatalf("clean close should not warn, logged: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "session reader exiting") {
		t.Fatalf("expected session reader exiting debug line, got: %s", buf.String())
	}
}

// TestReadPumpWarnsOnBrokenPipe tests that an unexpected read error warns.
func TestReadPumpWarnsOnBrokenPipe(t *testing.T) {
	var buf bytes.Buffer
	sh := newTestStreamHandler(errors.New("broken pipe"), &buf)
	sh.readPump(sh.ctx)
	if !strings.Contains(buf.String(), "error receiving message") {
		t.Fatalf("broken pipe should warn, logged: %s", buf.String())
	}
}
