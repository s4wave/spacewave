package transport_quic

import (
	"context"
	"io"
	"testing"

	"github.com/quic-go/quic-go"
)

func TestIsCleanAcceptClose(t *testing.T) {
	t.Run("zero application error", func(t *testing.T) {
		err := &quic.ApplicationError{ErrorCode: 0}
		if !isCleanAcceptClose(err) {
			t.Fatal("expected zero application error to be clean")
		}
	})

	t.Run("nonzero application error", func(t *testing.T) {
		err := &quic.ApplicationError{ErrorCode: 1}
		if isCleanAcceptClose(err) {
			t.Fatal("expected nonzero application error to remain exceptional")
		}
	})

	t.Run("existing eof classifications", func(t *testing.T) {
		for _, err := range []error{context.Canceled, io.EOF, io.ErrClosedPipe} {
			if !isCleanAcceptClose(err) {
				t.Fatalf("expected %v to be clean", err)
			}
		}
	})
}
