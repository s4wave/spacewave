// Package gstencoder reads hardware-encoded AVC from the Linux display helper.
package gstencoder

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/core/frametrack"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// stopGrace bounds a stop when a driver stalls before or during teardown.
const stopGrace = 2 * time.Second

// Source owns one helper process and implements the frame track's source.
type Source struct {
	// ctx ends when the source stops or its helper exits.
	ctx context.Context
	// stop closes both parent pipe ends through ctx.
	stop context.CancelCauseFunc
	// pictures stays open for the final buffered packets after a child exit.
	pictures *os.File
	// packets reads pictures and sends commands over the helper's pipes.
	packets *stream_packet.Session
	// keyframe coalesces outstanding requests without blocking a viewer.
	keyframe chan struct{}
	// exited holds the helper's wait result after pipe cleanup.
	exited *promise.Promise[struct{}]

	// mtx orders keyframe requests against the reader's delivery of pictures.
	mtx sync.Mutex
	// waitingIDR drops older deltas until an independently decodable picture arrives.
	waitingIDR bool
}

// OpenSource starts a test-pattern encoder and binds it to ctx.
// executable is a locally configured helper name or path.
func OpenSource(ctx context.Context, executable string) (*Source, error) {
	return openSource(ctx, executable, nil, &Capture{})
}

// OpenPipeWireSource starts capture of nodeID on the portal's restricted remote.
// The caller owns remote and can close it as soon as this call returns.
func OpenPipeWireSource(ctx context.Context, executable string, remote *os.File, nodeID uint32) (*Source, error) {
	return openSource(ctx, executable, []*os.File{remote}, &Capture{PipewireNodeId: nodeID})
}

// openSource starts the helper and sends its capture configuration before commands.
func openSource(ctx context.Context, executable string, remotes []*os.File, capture *Capture) (*Source, error) {
	// Hand the subprocess independent pipe ends so Wait cannot race a pipe read.
	input, commands, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer closePipe(input)
	pictures, output, err := os.Pipe()
	if err != nil {
		closePipe(commands)
		return nil, err
	}
	defer closePipe(output)

	// Give EOF time to stop GStreamer before forcefully ending a stalled child.
	ctx, stop := context.WithCancelCause(ctx)
	cmd := exec.CommandContext(ctx, executable) //nolint:gosec // The caller selects a local executable.
	cmd.Cancel = func() error {
		closePipe(commands)
		closePipe(pictures)
		return os.ErrProcessDone
	}
	cmd.WaitDelay = stopGrace
	cmd.Stdin, cmd.Stdout, cmd.Stderr = input, output, os.Stderr
	cmd.ExtraFiles = remotes
	if err := cmd.Start(); err != nil {
		cause := context.Cause(ctx)
		stop(err)
		closePipe(commands)
		closePipe(pictures)
		if cause != nil {
			return nil, cause
		}
		return nil, errors.Join(ErrEncoderFailed, err)
	}

	// Reap the helper before publishing the terminal result.
	source := &Source{
		ctx: ctx, stop: stop, pictures: pictures,
		packets: stream_packet.NewSession(struct {
			io.Reader
			io.WriteCloser
		}{pictures, commands}, MaxPacketSize),
		keyframe: make(chan struct{}, 1),
		exited:   promise.NewPromise[struct{}](),
	}
	go source.run(cmd, commands, capture)
	return source, nil
}

// CanRequestKeyframe reports that the hardware encoder accepts requests.
func (s *Source) CanRequestKeyframe() bool {
	return true
}

// ReadFrame reads one AVC access unit. Canceling the read stops the source.
func (s *Source) ReadFrame(ctx context.Context) (*frametrack.GroupHeader, *frametrack.Frame, error) {
	// Bind the pipe read to both the track and helper lifetime.
	stop := context.AfterFunc(ctx, func() { s.stop(context.Cause(ctx)) })
	defer stop()

	// Skip queued deltas when a viewer is waiting to start a new group.
	for {
		// Report deliberate shutdown or the encoder's terminal failure.
		packet := new(frametrack.FramePacket)
		err := s.packets.RecvMsg(packet)
		if ctx.Err() != nil {
			cause := context.Cause(ctx)
			s.stop(cause)
			return nil, nil, cause
		}
		if cause := context.Cause(s.ctx); cause != nil && cause != ErrEncoderFailed {
			return nil, nil, cause
		}
		if err != nil {
			s.stop(ErrEncoderFailed)
			_, exitErr := s.exited.Await(context.Background())
			return nil, nil, errors.Join(ErrEncoderFailed, err, exitErr)
		}

		// Do not let buffered deltas enter the track after a keyframe request.
		frame := packet.GetFrame()
		s.mtx.Lock()
		if frame.GetKeyframe() {
			s.waitingIDR = false
		}
		deliver := !s.waitingIDR
		s.mtx.Unlock()
		if deliver {
			return packet.GetHeader(), frame, nil
		}
	}
}

// RequestKeyframe schedules an IDR without waiting for the encoder.
func (s *Source) RequestKeyframe() {
	// Coalesce requests until the track can start its next group.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.waitingIDR {
		return
	}
	s.waitingIDR = true
	select {
	case s.keyframe <- struct{}{}:
	default:
	}
}

// Close stops and reaps the helper. ReadFrame reports capture failures.
func (s *Source) Close() error {
	// Keep shutdown synchronous so the track can replace a stopped source.
	s.stop(context.Canceled)
	_, _ = s.exited.Await(context.Background())
	closePipe(s.pictures)
	return nil
}

// run joins the process and the command writer. Close releases the picture pipe.
func (s *Source) run(cmd *exec.Cmd, commands *os.File, capture *Capture) {
	// Only the command worker can block on stdin writes.
	commandsDone := make(chan struct{})
	go func() {
		defer close(commandsDone)
		s.sendCommands(capture)
	}()

	// Retire the writer after exit. The reader can drain stdout through EOF.
	err := cmd.Wait()
	s.stop(ErrEncoderFailed)
	closePipe(commands)
	<-commandsDone
	s.exited.SetResult(struct{}{}, err)
}

// sendCommands forwards coalesced requests until the pipe or source ends.
func (s *Source) sendCommands(capture *Capture) {
	// Select the input before GStreamer opens any device or remote.
	if err := s.packets.SendMsg(capture); err != nil {
		s.stop(ErrEncoderFailed)
		return
	}

	// Deliver keyframe requests for the rest of this capture.
	cmd := &Command{RequestKeyframe: true}
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.keyframe:
			if err := s.packets.SendMsg(cmd); err != nil {
				s.stop(ErrEncoderFailed)
				return
			}
		}
	}
}

// closePipe retires an endpoint whose only possible close error is an OS error.
func closePipe(pipe *os.File) {
	_ = pipe.Close()
}

// _ is a type assertion.
var _ frametrack.Source = (*Source)(nil)
