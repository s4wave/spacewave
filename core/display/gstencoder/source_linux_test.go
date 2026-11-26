package gstencoder

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/frametrack"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// TestSourceGPU measures 1080p capture and requests IDRs between pictures.
func TestSourceGPU(t *testing.T) {
	// Run the same helper the display node starts when a GPU is available.
	executable := os.Getenv("SPACEWAVE_GST_TEST_HELPER")
	if executable == "" {
		t.Skip("set SPACEWAVE_GST_TEST_HELPER to the Linux GStreamer helper")
	}
	source, err := OpenSource(t.Context(), executable)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })

	// Read the opening AVC configuration and an actual IDR NAL unit.
	header, frame, err := source.ReadFrame(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	checkIDR(t, header, frame)
	first := time.Now()

	// Request mid-GOP IDRs and require the very next picture to carry each one.
	var maxLatency time.Duration
	for i := range 299 {
		// Ask for an IDR often enough to avoid the encoder's own GOP boundary.
		request := i%60 == 10
		requested := time.Now()
		if request {
			source.RequestKeyframe()
		}

		// Verify each request through the same protobuf reader the track uses.
		header, frame, err := source.ReadFrame(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if request {
			maxLatency = max(maxLatency, time.Since(requested))
			checkIDR(t, header, frame)
			continue
		}
		if frame.GetKeyframe() {
			t.Fatalf("unexpected keyframe at picture %d", i+1)
		}
	}
	t.Logf("1920x1080: %.2f frames/s, 5 requests, maximum next-picture IDR latency %s",
		299/time.Since(first).Seconds(), maxLatency)

	// Cancel the frame read and require the process to exit normally.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := source.ReadFrame(ctx); err != context.Canceled {
		t.Fatalf("canceled read returned %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := source.exited.Await(t.Context()); err != nil {
		t.Fatalf("encoder did not shut down normally: %v", err)
	}
}

// checkIDR checks the AVC description and scans the access unit for an IDR NAL.
func checkIDR(t *testing.T, header *frametrack.GroupHeader, frame *frametrack.Frame) {
	// Read the NAL length width from the hardware encoder's AVC description.
	t.Helper()
	config := header.GetDecoderDescription()
	if header.GetWidth() != 1920 || header.GetHeight() != 1080 ||
		header.GetCodec() != frametrack.Codec_CODEC_H264_AVC || len(config) < 7 || config[0] != 1 ||
		!frame.GetKeyframe() {
		t.Fatalf("expected a 1080p AVC keyframe with decoder configuration, got %v", header)
	}
	if config[4]&3 != 3 {
		t.Fatalf("expected four-byte NAL lengths, got %d", config[4]&3+1)
	}

	// An intra picture alone is insufficient. Find H.264's type-5 IDR NAL.
	for data := frame.GetData(); len(data) >= 4; {
		length := int(binary.BigEndian.Uint32(data[:4]))
		if length < 1 || length > len(data)-4 {
			t.Fatal("invalid AVC access unit")
		}
		if data[4]&0x1f == 5 {
			return
		}
		data = data[4+length:]
	}
	t.Fatal("keyframe contains no IDR NAL")
}

// TestSourceRecovery covers a queued delta, a crash, and a stalled shutdown.
func TestSourceRecovery(t *testing.T) {
	// Resume from an IDR after the encoder or its old pictures become unusable.
	t.Run("PictureWire", checkPictureWire)
	t.Run("QueuedDelta", checkSourceQueuedDelta)
	t.Run("Crash", checkSourceCrash)
	t.Run("Stalled", checkSourceStalled)
}

// checkPictureWire verifies the split writer against the generated protobuf.
func checkPictureWire(t *testing.T) {
	// Send one group-opening picture through the helper's actual pipe writer.
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	writer := NewPictureWriter(int(w.Fd()))
	packet := &frametrack.FramePacket{
		Header: &frametrack.GroupHeader{Codec: frametrack.Codec_CODEC_H264_AVC, DecoderDescription: []byte{1, 2}},
		Frame:  &frametrack.Frame{Keyframe: true, Data: []byte{3, 4}},
	}
	if err := writer.WriteFrame(packet.Header, packet.Frame); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	// Require the same bytes as the protobuf generator's whole-message encoder.
	encoded, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	want, err := packet.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) < 4 || int(binary.LittleEndian.Uint32(encoded)) != len(want) || !bytes.Equal(encoded[4:], want) {
		t.Fatalf("split protobuf = %x, want %x with length prefix", encoded, want)
	}

	// Decoding leaves the immutable packet's access unit in its original buffer.
	header, frame, err := decodePicture(encoded[4:])
	if err != nil || !packet.Header.EqualVT(header) || !packet.Frame.EqualVT(frame) {
		t.Fatalf("decoded packet = %v, %v, error %v", header, frame, err)
	}
	if &frame.Data[0] != &encoded[len(encoded)-len(frame.Data)] {
		t.Fatal("decoded access unit did not borrow packet bytes")
	}
}

// checkSourceQueuedDelta checks that the next delivered picture is an IDR.
func checkSourceQueuedDelta(t *testing.T) {
	// Buffer a delta that the helper wrote before it could receive the request.
	t.Helper()
	input, output, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close(); _ = output.Close() })
	packets := stream_packet.NewSession(struct {
		io.Reader
		io.WriteCloser
	}{WriteCloser: output}, MaxPacketSize)
	if err := packets.SendMsgs(
		&frametrack.FramePacket{Frame: &frametrack.Frame{Data: []byte{1}}},
		&frametrack.FramePacket{Frame: &frametrack.Frame{Keyframe: true, Data: []byte{2}}},
	); err != nil {
		t.Fatal(err)
	}

	// Exercise the actual source reader, including the request issued after the delta.
	ctx, stop := context.WithCancelCause(t.Context())
	defer stop(nil)
	source := &Source{ctx: ctx, stop: stop, keyframe: make(chan struct{}, 1), pictures: input}
	source.RequestKeyframe()
	_, frame, err := source.ReadFrame(t.Context())
	if err != nil || !frame.GetKeyframe() || !bytes.Equal(frame.GetData(), []byte{2}) {
		t.Fatalf("requested IDR returned frame %v and error %v", frame, err)
	}
}

// checkSourceCrash reports the failure of a subprocess that sends no pictures.
func checkSourceCrash(t *testing.T) {
	// The crashing fixture implements only the helper's process boundary.
	t.Helper()
	executable := filepath.Join(t.TempDir(), "crash")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 27\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	source, err := OpenSource(t.Context(), executable)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })

	// A helper exit is a source failure even when its picture pipe returns EOF.
	_, _, err = source.ReadFrame(t.Context())
	if !errors.Is(err, ErrEncoderFailed) || err == io.EOF {
		t.Fatalf("crash returned %v", err)
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 27 {
		t.Fatalf("crash lost its exit status: %v", err)
	}
}

// checkSourceStalled reaps a helper that cannot read its input pipe.
func checkSourceStalled(t *testing.T) {
	// Suspend the child to model a driver stall during initialization.
	t.Helper()
	executable := filepath.Join(t.TempDir(), "stall")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nkill -STOP $$\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	source, err := OpenSource(t.Context(), executable)
	if err != nil {
		t.Fatal(err)
	}

	// Close must reap the child even when EOF cannot wake it.
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = source.exited.Await(t.Context())
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatalf("stalled encoder was not killed and reaped: %v", err)
	}
}
