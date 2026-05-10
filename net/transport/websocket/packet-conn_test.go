package websocket

import (
	"context"
	"io"
	"testing"

	gowebsocket "github.com/aperturerobotics/go-websocket"
	"github.com/pkg/errors"
)

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
