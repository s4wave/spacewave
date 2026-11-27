package websocket

import (
	"context"
	"io"
	"testing"
	"time"

	gowebsocket "github.com/aperturerobotics/go-websocket"
	"github.com/pkg/errors"
)

// TestIsCleanReadClose checks normal closures and protocol failures.
func TestIsCleanReadClose(t *testing.T) {
	t.Run("going away", func(t *testing.T) {
		err := errors.Wrap(gowebsocket.CloseError{
			Code: gowebsocket.StatusGoingAway,
		}, "failed to get reader")
		if !isCleanReadClose(err) {
			t.Fatal("expected StatusGoingAway to be clean")
		}
	})

	t.Run("normal closure", func(t *testing.T) {
		err := errors.Wrap(gowebsocket.CloseError{
			Code: gowebsocket.StatusNormalClosure,
		}, "failed to get reader")
		if !isCleanReadClose(err) {
			t.Fatal("expected StatusNormalClosure to be clean")
		}
	})

	t.Run("existing eof classifications", func(t *testing.T) {
		for _, err := range []error{context.Canceled, io.EOF} {
			if !isCleanReadClose(err) {
				t.Fatalf("expected %v to be clean", err)
			}
		}
	})

	t.Run("protocol error", func(t *testing.T) {
		err := errors.Wrap(gowebsocket.CloseError{
			Code: gowebsocket.StatusProtocolError,
		}, "failed to get reader")
		if isCleanReadClose(err) {
			t.Fatal("expected StatusProtocolError to remain exceptional")
		}
	})
}

// TestPacketConnConcurrentDeadlines checks that independent QUIC shutdown paths
// can replace both deadlines concurrently without racing.
func TestPacketConnConcurrentDeadlines(t *testing.T) {
	// Deadline publication needs no WebSocket backend or network I/O.
	conn := NewPacketConn(t.Context(), nil, nil, nil)
	setters := []func(time.Time) error{conn.SetDeadline, conn.SetReadDeadline, conn.SetWriteDeadline}
	start := make(chan struct{})
	done := make(chan error, len(setters))

	// Release competing setters together, including clearing each deadline.
	for _, set := range setters {
		go func() {
			<-start
			for range 1000 {
				if err := set(time.Now().Add(time.Hour)); err != nil {
					done <- err
					return
				}
				if err := set(time.Time{}); err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}()
	}
	close(start)

	// Join all setters before the test releases the connection state.
	for range setters {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent deadline setters did not return")
		}
	}
}

// BenchmarkPacketConnDeadlines measures deadline publication without network I/O.
func BenchmarkPacketConnDeadlines(b *testing.B) {
	// Compare disabling deadlines with publishing a future snapshot.
	for _, tc := range []struct {
		// name identifies the publication state.
		name string
		// deadline is zero or a future deadline.
		deadline time.Time
	}{{"clear", time.Time{}}, {"future", time.Now().Add(time.Hour)}} {
		b.Run(tc.name, func(b *testing.B) {
			// Deadline publication does not call the absent WebSocket backend.
			conn := NewPacketConn(b.Context(), nil, nil, nil)
			b.ReportAllocs()
			for b.Loop() {
				if err := conn.SetDeadline(tc.deadline); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
