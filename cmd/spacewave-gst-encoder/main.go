//go:build linux && cgo && gstreamer

// Command spacewave-gst-encoder runs the Linux display's GStreamer pipeline.
package main

/*
#cgo pkg-config: gstreamer-app-1.0 gstreamer-video-1.0
#include "pipeline.h"
*/
import "C"

import (
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
	"unsafe"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/display/gstencoder"
	"github.com/s4wave/spacewave/core/frametrack"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"golang.org/x/sys/unix"
)

// main keeps all diagnostics on stderr so stdout carries only pictures.
func main() {
	if err := run(); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

// run binds the encoder to both ends of the subprocess connection.
func run() error {
	// Register the control descriptor with Go's poller so closing wakes its reader.
	if err := unix.SetNonblock(unix.Stdin, true); err != nil {
		return err
	}
	input := os.NewFile(uintptr(unix.Stdin), "encoder commands")

	// Return a write error when the parent closes stdout so the pipeline can stop.
	signal.Ignore(syscall.SIGPIPE)
	packets := stream_packet.NewSession(struct {
		io.Reader
		io.WriteCloser
	}{input, os.Stdout}, gstencoder.MaxPacketSize)

	// The daemon chooses the source and already owns the portal FD, if any.
	var capture gstencoder.Capture
	if err := packets.RecvMsg(&capture); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return errors.Wrap(err, "read encoder capture configuration")
	}

	// Open the encoder and start listening for commands.
	var failure *C.GError
	pipeline := C.open_pipeline(C.guint32(capture.GetPipewireNodeId()), &failure)
	if failure != nil {
		return gstError(failure)
	}
	defer C.close_pipeline(pipeline)
	commands := make(chan error, 1)
	go func() { commands <- readCommands(pipeline, packets) }()

	// Close stdin and join the command worker before releasing the C pipeline.
	err := writePictures(pipeline, packets)
	_ = input.Close()
	commandErr := <-commands
	if errors.Is(err, syscall.EPIPE) {
		return nil
	}
	if err != nil {
		return err
	}
	return commandErr
}

// readCommands requests IDRs and stops the pipeline when the input closes.
func readCommands(pipeline *C.Pipeline, packets *stream_packet.Session) error {
	defer C.stop_pipeline(pipeline)
	for {
		// A closed input means the Device canceled capture.
		var cmd gstencoder.Command
		if err := packets.RecvMsg(&cmd); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
				return nil
			}
			return err
		}

		// Ask the encoder directly so the next input picture can become an IDR.
		if cmd.GetRequestKeyframe() && C.request_idr(pipeline) == 0 {
			return errors.New("GStreamer encoder rejected force-key-unit event")
		}
	}
}

// writePictures forwards each completed access unit as one length-prefixed packet.
func writePictures(pipeline *C.Pipeline, packets *stream_packet.Session) error {
	for {
		// The bus wakes on a new sample, pipeline failure, or closed command pipe.
		var failure *C.GError
		sample := C.read_sample(pipeline, &failure)
		if failure != nil {
			return gstError(failure)
		}
		if sample == nil {
			return nil
		}

		// Hold the sample until its bytes have been serialized.
		err := writePicture(pipeline, sample, packets)
		C.gst_sample_unref(sample)
		if err != nil {
			return err
		}
	}
}

// writePicture borrows GStreamer memory through the synchronous protobuf write.
func writePicture(pipeline *C.Pipeline, sample *C.GstSample, packets *stream_packet.Session) error {
	// Map the coded access unit and, for an IDR, the caps' decoder description.
	var pic C.Picture
	if C.map_picture(pipeline, sample, &pic) == 0 {
		return errors.New("map GStreamer picture or AVC decoder description")
	}
	defer C.unmap_picture(&pic)

	// Carry capture time and enough information for a decoder to start at this IDR.
	keyframe := pic.keyframe != 0
	frame := &frametrack.Frame{
		CapturedAt: timestamp.New(time.Now().Add(-time.Duration(pic.age))),
		Keyframe:   keyframe,
		Data:       unsafe.Slice((*byte)(pic.bytes.data), int(pic.bytes.size)),
	}
	packet := &frametrack.FramePacket{Frame: frame}
	if keyframe {
		packet.Header = &frametrack.GroupHeader{
			Codec:              frametrack.Codec_CODEC_H264_AVC,
			Width:              uint32(pic.width),
			Height:             uint32(pic.height),
			DecoderDescription: unsafe.Slice((*byte)(pic.avcc.data), int(pic.avcc.size)),
		}
	}

	// The helper and frame track share the same bound on access units.
	if packet.SizeVT() > gstencoder.MaxPacketSize {
		return stream_packet.ErrMessageTooLarge
	}
	return packets.SendMsg(packet)
}

// gstError copies the pipeline's diagnostic and frees its C allocation.
func gstError(failure *C.GError) error {
	defer C.g_error_free(failure)
	return errors.New(C.GoString(failure.message))
}
