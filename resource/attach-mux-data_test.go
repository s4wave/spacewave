package resource

import (
	"io"
	"testing"

	"github.com/pkg/errors"
)

func TestAttachMuxDataRwc_Read_BasicData(t *testing.T) {
	data := []byte("hello world")
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return nil },
		func() ([]byte, error) { return data, nil },
	)

	buf := make([]byte, 64)
	n, err := rwc.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("read %d bytes, want %d", n, len(data))
	}
	if string(buf[:n]) != "hello world" {
		t.Fatalf("got %q, want %q", string(buf[:n]), "hello world")
	}
}

func TestAttachMuxDataRwc_Read_BuffersPartialReads(t *testing.T) {
	called := false
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return nil },
		func() ([]byte, error) {
			if called {
				t.Fatal("recvMuxData called more than once")
			}
			called = true
			return []byte("abcdef"), nil
		},
	)

	buf := make([]byte, 3)
	n, err := rwc.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error on first read: %v", err)
	}
	if n != 3 || string(buf[:n]) != "abc" {
		t.Fatalf("first read: got %q, want %q", string(buf[:n]), "abc")
	}

	n, err = rwc.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error on second read: %v", err)
	}
	if n != 3 || string(buf[:n]) != "def" {
		t.Fatalf("second read: got %q, want %q", string(buf[:n]), "def")
	}
}

func TestAttachMuxDataRwc_Read_SkipsEmptyData(t *testing.T) {
	calls := 0
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return nil },
		func() ([]byte, error) {
			calls++
			switch calls {
			case 1:
				return nil, nil
			case 2:
				return []byte{}, nil
			case 3:
				return []byte("data"), nil
			default:
				t.Fatal("too many recv calls")
				return nil, nil
			}
		},
	)

	buf := make([]byte, 64)
	n, err := rwc.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 recv calls, got %d", calls)
	}
	if string(buf[:n]) != "data" {
		t.Fatalf("got %q, want %q", string(buf[:n]), "data")
	}
}

func TestAttachMuxDataRwc_Read_ReturnsRecvError(t *testing.T) {
	recvErr := errors.New("recv failed")
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return nil },
		func() ([]byte, error) { return nil, recvErr },
	)

	buf := make([]byte, 64)
	_, err := rwc.Read(buf)
	if err != recvErr {
		t.Fatalf("got error %v, want %v", err, recvErr)
	}
}

func TestAttachMuxDataRwc_Write_SendsData(t *testing.T) {
	var sent []byte
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error {
			sent = d
			return nil
		},
		func() ([]byte, error) { return nil, nil },
	)

	input := []byte("payload")
	n, err := rwc.Write(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(input) {
		t.Fatalf("wrote %d bytes, want %d", n, len(input))
	}
	if string(sent) != "payload" {
		t.Fatalf("sent %q, want %q", string(sent), "payload")
	}
}

func TestAttachMuxDataRwc_Write_ReturnsLength(t *testing.T) {
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return nil },
		func() ([]byte, error) { return nil, nil },
	)

	data := []byte("twelve bytes")
	n, err := rwc.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("got %d, want %d", n, len(data))
	}
}

func TestAttachMuxDataRwc_Write_PropagatesError(t *testing.T) {
	sendErr := errors.New("send failed")
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return sendErr },
		func() ([]byte, error) { return nil, nil },
	)

	_, err := rwc.Write([]byte("data"))
	if err != sendErr {
		t.Fatalf("got error %v, want %v", err, sendErr)
	}
}

func TestAttachMuxDataRwc_Write_ClonesData(t *testing.T) {
	var sent []byte
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error {
			sent = d
			return nil
		},
		func() ([]byte, error) { return nil, nil },
	)

	input := []byte("original")
	_, err := rwc.Write(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mutate the original buffer after Write.
	input[0] = 'X'

	// The sent data should still have the original value.
	if sent[0] != 'o' {
		t.Fatalf("sent data was mutated: got %q, want first byte 'o'", string(sent))
	}
}

func TestAttachMuxDataRwc_Close_ReturnsNil(t *testing.T) {
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return nil },
		func() ([]byte, error) { return nil, nil },
	)

	if err := rwc.Close(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestAttachMuxDataRwc_ImplementsReadWriteCloser(t *testing.T) {
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return nil },
		func() ([]byte, error) { return nil, nil },
	)

	var _ io.ReadWriteCloser = rwc
}
