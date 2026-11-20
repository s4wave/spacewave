package frametrack

import (
	"context"
	"io"
	"net"
	"testing"
	"testing/synctest"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

// TestSenderDropsDeltaBeforeKeyframe rules out the delta-opens-group mutant.
func TestSenderDropsDeltaBeforeKeyframe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Create a pass-through subscription without an open group.
		opened := false
		sender, err := NewSender(t.Context(), "viewer", 2, func(context.Context) (io.ReadWriteCloser, error) {
			opened = true
			return nil, io.ErrClosedPipe
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer sender.Close()

		// A delta cannot cause a group open or enter an unwritten group.
		if sender.Send(nil, &Frame{Data: []byte{1}}) {
			t.Fatal("sender accepted a delta before the first keyframe")
		}
		synctest.Wait()
		if opened {
			t.Fatal("sender opened a stream for a delta")
		}
	})
}

// TestSenderRecoversFromFullQueue rules out the drop-leaves-group-open mutant.
func TestSenderRecoversFromFullQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Open groups on pipes whose writes wait for this test to read.
		streams := make(chan net.Conn, 2)
		requests := 0
		sender, err := NewSender(t.Context(), "viewer", 2, openTestGroup(streams), func() { requests++ })
		if err != nil {
			t.Fatal(err)
		}
		defer sender.Close()

		// Read the header and hold the first frame inside its blocked write.
		header := &GroupHeader{
			Codec: Codec_CODEC_H264_AVC, Width: 640, Height: 360,
			DecoderDescription: []byte{1, 2, 3},
		}
		key := &Frame{CapturedAt: &timestamp.Timestamp{Seconds: 1}, Keyframe: true, Data: []byte{1}}
		if !sender.Send(header, key) {
			t.Fatal("sender rejected the initial keyframe")
		}
		first := readTestGroup(t, streams)
		wantHeader := header.CloneVT()
		wantHeader.SubscriptionId = "viewer"
		wantHeader.GroupSequence = 1
		if !first.Header().EqualVT(wantHeader) {
			t.Fatalf("first group header = %v, want %v", first.Header(), wantHeader)
		}
		synctest.Wait()

		// Fill the remaining slot and require a keyframe request on overflow.
		if !sender.Send(nil, &Frame{Data: []byte{2}}) {
			t.Fatal("sender rejected a delta with one slot available")
		}
		if sender.Send(nil, &Frame{Data: []byte{3}}) {
			t.Fatal("sender accepted a delta past the window")
		}
		if requests != 1 {
			t.Fatalf("overflow triggered %d keyframe requests, want 1", requests)
		}

		// Free a slot while the accepted delta remains blocked. A later delta
		// would leave a decode gap even though a slot is now available.
		readTestFrame(t, first, key)
		synctest.Wait()
		if sender.Send(nil, &Frame{Data: []byte{4}}) {
			t.Fatal("sender accepted a delta after the group was sealed")
		}
		if requests != 1 {
			t.Fatalf("sealed group triggered %d keyframe requests, want 1", requests)
		}

		// A replacement keyframe opens a new stream while the old write waits.
		next := &Frame{CapturedAt: &timestamp.Timestamp{Seconds: 2}, Keyframe: true, Data: []byte{5}}
		if !sender.Send(header, next) {
			t.Fatal("sender rejected the recovery keyframe")
		}
		second := readTestGroup(t, streams)
		if got := second.Header().GetGroupSequence(); got != 2 {
			t.Fatalf("replacement group sequence = %d, want 2", got)
		}
		readTestFrame(t, second, next)

		// Replacing the old group cancels its blocked write and discards its tail.
		synctest.Wait()
		if _, err := first.ReadFrame(); err != io.EOF {
			t.Fatalf("reading replaced group's tail returned %v, want EOF", err)
		}
	})
}

// TestSenderRecoversFromFailedOpen treats a failed group open as a drop.
func TestSenderRecoversFromFailedOpen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Make the first open fail and allow the next to reach the viewer.
		streams := make(chan net.Conn, 1)
		open := openTestGroup(streams)
		attempts, requests := 0, 0
		sender, err := NewSender(t.Context(), "viewer", 2, func(ctx context.Context) (io.ReadWriteCloser, error) {
			attempts++
			if attempts == 1 {
				return nil, io.ErrClosedPipe
			}
			return open(ctx)
		}, func() { requests++ })
		if err != nil {
			t.Fatal(err)
		}
		defer sender.Close()

		// A failed open discards the group and requests its replacement.
		header := &GroupHeader{Codec: Codec_CODEC_JPEG, Width: 640, Height: 360}
		key := &Frame{Keyframe: true, Data: []byte{1}}
		sender.Send(header, key)
		synctest.Wait()
		if requests != 1 {
			t.Fatalf("failed open triggered %d keyframe requests, want 1", requests)
		}
		if sender.Send(nil, &Frame{Data: []byte{2}}) {
			t.Fatal("sender accepted a delta after an open failure")
		}
		if attempts != 1 {
			t.Fatalf("open attempts = %d, want 1 until the next keyframe", attempts)
		}

		// The next keyframe advances the sequence and begins a decodable group.
		sender.Send(header, key)
		group := readTestGroup(t, streams)
		if got := group.Header().GetGroupSequence(); got != 2 {
			t.Fatalf("recovery group sequence = %d, want 2", got)
		}
		readTestFrame(t, group, key)
	})
}

// openTestGroup hands each receiver a pipe that controls its writer's progress.
func openTestGroup(streams chan<- net.Conn) OpenGroupStream {
	return func(context.Context) (io.ReadWriteCloser, error) {
		send, recv := net.Pipe()
		streams <- recv
		return send, nil
	}
}

// readTestGroup receives a group stream and reads its header.
func readTestGroup(t *testing.T, streams <-chan net.Conn) *GroupReader {
	// Take ownership of the group stream's receive end.
	t.Helper()
	synctest.Wait()
	var strm net.Conn
	select {
	case strm = <-streams:
	default:
		t.Fatal("sender did not open a group")
	}
	t.Cleanup(func() { _ = strm.Close() })

	// Start reading the frames only after their format has arrived.
	group, err := NewGroupReader(strm)
	if err != nil {
		t.Fatal(err)
	}
	return group
}

// readTestFrame verifies the picture that crosses one group stream.
func readTestFrame(t *testing.T, group *GroupReader, want *Frame) {
	t.Helper()
	got, err := group.ReadFrame()
	if err != nil || !got.EqualVT(want) {
		t.Fatalf("read frame %v, error %v, want %v", got, err, want)
	}
}
