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
	err := writePictures(pipeline, gstencoder.NewPictureWriter(int(os.Stdout.Fd())))
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
func writePictures(pipeline *C.Pipeline, writer *gstencoder.PictureWriter) error {
	// Reuse both the cgo mapping and the protobuf metadata across frames.
	picture := encoderPicture{}
	picture.frame.CapturedAt = &picture.captured
	picture.header.Codec = frametrack.Codec_CODEC_H264_AVC
	var failure *C.GError

	// Forward each completed sample before releasing its GStreamer reference.
	for {
		// The bus wakes on a new sample, pipeline failure, or closed command pipe.
		sample := C.read_sample(pipeline, &failure)
		if failure != nil {
			return gstError(failure)
		}
		if sample == nil {
			return nil
		}

		// Hold the sample until its bytes have been serialized.
		err := picture.write(pipeline, sample, writer)
		C.gst_sample_unref(sample)
		if err != nil {
			return err
		}
	}
}

// encoderPicture owns the metadata reused by the encoder's single writer.
type encoderPicture struct {
	// mapped borrows one GStreamer sample during its synchronous write.
	mapped C.Picture
	// header carries the decoder configuration for a keyframe.
	header frametrack.GroupHeader
	// frame points into mapped for the duration of the write.
	frame frametrack.Frame
	// captured holds the timestamp referenced by frame.
	captured timestamp.Timestamp
}

// write borrows the coded picture and AVC description through the pipe write.
func (p *encoderPicture) write(pipeline *C.Pipeline, sample *C.GstSample, writer *gstencoder.PictureWriter) error {
	// Reset the map so a delta cannot inherit the previous frame's description.
	p.mapped = C.Picture{}
	if C.map_picture(pipeline, sample, &p.mapped) == 0 {
		return errors.New("map GStreamer picture or AVC decoder description")
	}
	defer func() {
		p.frame.Data = nil
		p.header.DecoderDescription = nil
		C.unmap_picture(&p.mapped)
	}()

	// Write the current picture and its capture time in the reusable metadata.
	at := time.Now().Add(-time.Duration(p.mapped.age))
	p.captured.Seconds = at.Unix()
	p.captured.Nanos = int32(at.Nanosecond()) //nolint:gosec // The subsecond nanosecond count fits int32.
	p.frame.Keyframe = p.mapped.keyframe != 0
	p.frame.Data = unsafe.Slice((*byte)(p.mapped.bytes.data), int(p.mapped.bytes.size))
	if !p.frame.Keyframe {
		return writer.WriteFrame(nil, &p.frame)
	}

	// A keyframe carries the decoder configuration until the pipe accepts it.
	p.header.Width = uint32(p.mapped.width)
	p.header.Height = uint32(p.mapped.height)
	p.header.DecoderDescription = unsafe.Slice((*byte)(p.mapped.avcc.data), int(p.mapped.avcc.size))
	return writer.WriteFrame(&p.header, &p.frame)
}

// gstError copies the pipeline's diagnostic and frees its C allocation.
func gstError(failure *C.GError) error {
	defer C.g_error_free(failure)
	return errors.New(C.GoString(failure.message))
}
