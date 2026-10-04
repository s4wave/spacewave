package resource

import (
	"io"
	"testing"

	"github.com/pkg/errors"
)

func TestAttachMuxDataRwc_Read_BasicData(t *testing.T) {
	// Prepare a mux reader that returns the complete payload.
	data := []byte("hello world")
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return nil },
		func() ([]byte, error) { return data, nil },
	)

	// Read the payload into a destination larger than the data.
	buf := make([]byte, 64)
	n, err := rwc.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert that the read returned every payload byte.
	if n != len(data) {
		t.Fatalf("read %d bytes, want %d", n, len(data))
	}

	// Assert that the destination contains the expected payload.
	if string(buf[:n]) != "hello world" {
		t.Fatalf("got %q, want %q", string(buf[:n]), "hello world")
	}
}

func TestAttachMuxDataRwc_Read_BuffersPartialReads(t *testing.T) {
	// Guard the receiver callback against extra reads.
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

	// Read the first chunk from the mux reader.
	buf := make([]byte, 3)
	n, err := rwc.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error on first read: %v", err)
	}

	// Assert that the first read returned the first three bytes.
	if n != 3 || string(buf[:n]) != "abc" {
		t.Fatalf("first read: got %q, want %q", string(buf[:n]), "abc")
	}

	// Read the buffered remainder without calling the receiver again.
	n, err = rwc.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error on second read: %v", err)
	}

	// Assert that the second read returned the remaining bytes.
	if n != 3 || string(buf[:n]) != "def" {
		t.Fatalf("second read: got %q, want %q", string(buf[:n]), "def")
	}
}

func TestAttachMuxDataRwc_Read_SkipsEmptyData(t *testing.T) {
	// Return empty payloads before the non-empty mux data.
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

	// Read through the empty responses to the first data chunk.
	buf := make([]byte, 64)
	n, err := rwc.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert that the reader skipped both empty responses.
	if calls != 3 {
		t.Fatalf("expected 3 recv calls, got %d", calls)
	}

	// Assert that the reader returned the non-empty payload.
	if string(buf[:n]) != "data" {
		t.Fatalf("got %q, want %q", string(buf[:n]), "data")
	}
}

func TestAttachMuxDataRwc_Read_ReturnsRecvError(t *testing.T) {
	// Configure the receiver callback to return a sentinel error.
	recvErr := errors.New("recv failed")
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return nil },
		func() ([]byte, error) { return nil, recvErr },
	)

	// Read once and capture the receiver error.
	buf := make([]byte, 64)
	_, err := rwc.Read(buf)

	// Assert that Read returns the receiver error unchanged.
	if err != recvErr {
		t.Fatalf("got error %v, want %v", err, recvErr)
	}
}

func TestAttachMuxDataRwc_Write_SendsData(t *testing.T) {
	// Capture the bytes that the mux writer sends.
	var sent []byte
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error {
			sent = d
			return nil
		},
		func() ([]byte, error) { return nil, nil },
	)

	// Write a payload and check the number of bytes reported.
	input := []byte("payload")
	n, err := rwc.Write(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert that Write reports the full input length.
	if n != len(input) {
		t.Fatalf("wrote %d bytes, want %d", n, len(input))
	}

	// Assert that the mux writer received the payload.
	if string(sent) != "payload" {
		t.Fatalf("sent %q, want %q", string(sent), "payload")
	}
}

func TestAttachMuxDataRwc_Write_ReturnsLength(t *testing.T) {
	// Construct a mux writer whose send callback succeeds.
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return nil },
		func() ([]byte, error) { return nil, nil },
	)

	// Write data and verify the reported byte count.
	data := []byte("twelve bytes")
	n, err := rwc.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert that Write reports the full data length.
	if n != len(data) {
		t.Fatalf("got %d, want %d", n, len(data))
	}
}

func TestAttachMuxDataRwc_Write_PropagatesError(t *testing.T) {
	// Configure the sender callback to return a sentinel error.
	sendErr := errors.New("send failed")
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return sendErr },
		func() ([]byte, error) { return nil, nil },
	)

	// Write through the callback that returns the sentinel error.
	_, err := rwc.Write([]byte("data"))

	// Assert that Write returns the sender error unchanged.
	if err != sendErr {
		t.Fatalf("got error %v, want %v", err, sendErr)
	}
}

func TestAttachMuxDataRwc_Write_ClonesData(t *testing.T) {
	// Capture the bytes retained by the mux writer.
	var sent []byte
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error {
			sent = d
			return nil
		},
		func() ([]byte, error) { return nil, nil },
	)

	// Write the original payload before changing its backing bytes.
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
	// Construct a mux stream with successful no-op callbacks.
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return nil },
		func() ([]byte, error) { return nil, nil },
	)

	// Close the stream and check its result.
	if err := rwc.Close(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestAttachMuxDataRwc_ImplementsReadWriteCloser(t *testing.T) {
	// Construct a mux stream for the interface assertion.
	rwc := NewAttachMuxDataRwc(
		func(d []byte) error { return nil },
		func() ([]byte, error) { return nil, nil },
	)

	// Verify the mux stream satisfies io.ReadWriteCloser.
	var _ io.ReadWriteCloser = rwc
}
