//go:build linux

// Package v4l2 captures compressed frames from a Linux video device.
package v4l2

import (
	"bytes"
	"context"
	"encoding/binary"
	stderrors "errors"
	"io"
	"sync"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/frametrack"
	"golang.org/x/sys/unix"
)

// Source streams compressed pictures from one V4L2 device.
// One caller reads frames in capture order.
type Source struct {
	// fd identifies the open capture device.
	fd int
	// wake signals cancellation or a keyframe request to the reader.
	wake int
	// buffers are the driver's mapped capture buffers, in index order.
	buffers [][]byte
	// streaming records whether the driver owns the queued buffers.
	streaming bool
	// header holds the negotiated codec and dimensions.
	header *frametrack.GroupHeader
	// avc converts captured H.264 pictures and tracks their dependencies.
	avc avcStream
	// forceKeyframe reports whether the device accepted the control query.
	forceKeyframe bool

	// readMtx keeps Close from unmapping buffers while a reader uses them.
	readMtx sync.Mutex
	// mtx guards shutdown and keeps a wake write from racing descriptor reuse.
	mtx sync.Mutex
	// closed prevents new reads and wake writes after shutdown begins.
	closed bool
	// keyframeRequested coalesces requests until the capture loop acts on one.
	keyframeRequested bool
}

// NewSource starts mmap capture, preferring H.264 over MJPEG.
func NewSource(path string) (_ frametrack.Source, err error) {
	// Open the camera and reserve a wake descriptor for cancellation.
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errors.Wrap(err, path)
	}
	wake, err := unix.Eventfd(0, unix.EFD_NONBLOCK|unix.EFD_CLOEXEC)
	if err != nil {
		return nil, stderrors.Join(err, unix.Close(fd))
	}
	s := &Source{fd: fd, wake: wake}
	defer func() {
		if err != nil {
			err = stderrors.Join(err, s.Close())
		}
	}()

	// Ask the driver for compressed capture at its current size.
	s.header, err = selectFormat(fd)
	if err != nil {
		return nil, errors.Wrap(err, path)
	}

	// Give the driver its mapped buffers and start capture.
	if err = s.start(); err != nil {
		return nil, errors.Wrap(err, path)
	}
	s.forceKeyframe = s.header.Codec == frametrack.Codec_CODEC_H264_AVC && canForceKeyframe(fd)
	return s, nil
}

// CanRequestKeyframe reports whether the device offers hardware keyframe control.
func (s *Source) CanRequestKeyframe() bool {
	return s.forceKeyframe
}

// ReadFrame waits for a decodable picture and returns bytes independent of the driver.
func (s *Source) ReadFrame(ctx context.Context) (*frametrack.GroupHeader, *frametrack.Frame, error) {
	// Keep the mapping live until this read has finished.
	s.readMtx.Lock()
	defer s.readMtx.Unlock()
	stop := context.AfterFunc(ctx, s.signal)
	defer stop()

	// Wait for a frame, cancellation, or shutdown without a deadline timer.
	fds := []unix.PollFd{
		{Fd: int32(s.fd), Events: unix.POLLIN},   //nolint:gosec // Linux file descriptors are signed 32-bit ints.
		{Fd: int32(s.wake), Events: unix.POLLIN}, //nolint:gosec // Linux file descriptors are signed 32-bit ints.
	}
	for {
		// Check shutdown and cancellation before waiting for the driver.
		s.mtx.Lock()
		closed, requested := s.closed, s.keyframeRequested
		s.keyframeRequested = false
		s.mtx.Unlock()
		if closed {
			return nil, nil, io.EOF
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if requested {
			// A driver that refuses the request still supplies its natural IDRs.
			ctrl := setControl{id: forceKeyFrameControl}
			_ = ioctl(s.fd, ioctlReadWrite, 28, &ctrl)
		}

		// Block on the device and the wake descriptor.
		if _, err := unix.Poll(fds, -1); err != nil {
			if err == unix.EINTR {
				continue
			}
			return nil, nil, errors.Wrap(err, "wait for camera")
		}

		// Consume wake events, including cancellation from an earlier read.
		if fds[1].Revents != 0 {
			var value [8]byte
			if _, err := unix.Read(s.wake, value[:]); err != nil && err != unix.EAGAIN && err != unix.EINTR {
				return nil, nil, errors.Wrap(err, "read camera wake event")
			}
			continue
		}
		if fds[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return nil, nil, errors.New("camera capture stopped")
		}

		// Take one completed buffer, then return it to the driver.
		header, frame, err := s.dequeue()
		if err != nil {
			return nil, nil, err
		}
		if frame != nil {
			return header, frame, nil
		}
	}
}

// RequestKeyframe wakes the capture loop to request an IDR when supported.
func (s *Source) RequestKeyframe() {
	// JPEG and cameras without the control use their own keyframe cadence.
	if !s.forceKeyframe {
		return
	}

	// Let the capture loop submit the ioctl without blocking a viewer.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if !s.closed && !s.keyframeRequested {
		s.keyframeRequested = true
		s.wakeReader()
	}
}

// Close interrupts a pending read, stops capture, and releases the device.
func (s *Source) Close() error {
	// Wake the reader before waiting for it to release its buffer.
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return nil
	}
	s.closed = true
	s.wakeReader()
	s.mtx.Unlock()

	// Stop the driver before unmapping its capture buffers.
	s.readMtx.Lock()
	defer s.readMtx.Unlock()
	var err error
	if s.streaming {
		typ := uint32(captureType)
		err = ioctl(s.fd, ioctlWrite, 19, &typ)
	}
	for _, buf := range s.buffers {
		err = stderrors.Join(err, unix.Munmap(buf))
	}
	return stderrors.Join(err, unix.Close(s.fd), unix.Close(s.wake))
}

// start maps and queues the buffers before transferring them to the driver.
func (s *Source) start() error {
	// Let the driver pick the number of buffers it can provide.
	req := requestBuffers{count: 4, typ: captureType, memory: mmapMemory}
	if err := ioctl(s.fd, ioctlReadWrite, 8, &req); err != nil {
		return errors.Wrap(err, "allocate camera buffers")
	}
	if req.count == 0 {
		return errors.New("camera allocated no capture buffers")
	}
	s.buffers = make([][]byte, 0, req.count)

	// Map each buffer using its driver-supplied offset, then make it available.
	for index := range req.count {
		// Query and map the indexed buffer before queueing it.
		buf := captureBuffer{index: index, typ: captureType, memory: mmapMemory}
		if err := ioctl(s.fd, ioctlReadWrite, 9, &buf); err != nil {
			return errors.Wrap(err, "query camera buffer")
		}
		mapped, err := unix.Mmap(s.fd, int64(buf.offset), int(buf.length), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
		if err != nil {
			return errors.Wrap(err, "map camera buffer")
		}
		s.buffers = append(s.buffers, mapped)

		// Transfer ownership of the mapping to the camera until it has a frame.
		if err := ioctl(s.fd, ioctlReadWrite, 15, &buf); err != nil {
			return errors.Wrap(err, "queue camera buffer")
		}
	}

	// Enable capture once the camera has all its buffers.
	typ := uint32(captureType)
	if err := ioctl(s.fd, ioctlWrite, 18, &typ); err != nil {
		return errors.Wrap(err, "start camera capture")
	}
	s.streaming = true
	return nil
}

// dequeue builds one frame before the driver may overwrite its mapping.
func (s *Source) dequeue() (*frametrack.GroupHeader, *frametrack.Frame, error) {
	// The driver can report readiness before a buffer is available.
	buf := captureBuffer{typ: captureType, memory: mmapMemory}
	err := ioctl(s.fd, ioctlReadWrite, 17, &buf)
	if err == unix.EAGAIN {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, errors.Wrap(err, "dequeue camera buffer")
	}

	// Bound the payload by the mapped buffer before decoding its metadata.
	if int64(buf.index) >= int64(len(s.buffers)) || int64(buf.used) > int64(len(s.buffers[buf.index])) {
		return nil, nil, errors.New("camera returned an invalid capture buffer")
	}
	header, frame := s.makeFrame(buf, s.buffers[buf.index][:buf.used])

	// Allow the next capture to use this buffer after the frame owns its bytes.
	buf = captureBuffer{index: buf.index, typ: captureType, memory: mmapMemory}
	if err := ioctl(s.fd, ioctlReadWrite, 15, &buf); err != nil {
		return nil, nil, errors.Wrap(err, "return camera buffer")
	}
	return header, frame, nil
}

// makeFrame copies a complete JPEG or converts one H.264 access unit.
func (s *Source) makeFrame(buf captureBuffer, data []byte) (*frametrack.GroupHeader, *frametrack.Frame) {
	// JPEG pictures can recover independently following a damaged buffer.
	damaged := len(data) == 0 || buf.flags&bufferError != 0
	if s.header.Codec == frametrack.Codec_CODEC_JPEG {
		if damaged {
			return nil, nil
		}
		return s.header, &frametrack.Frame{
			CapturedAt: timestamp.New(time.Now()),
			Keyframe:   true,
			Data:       bytes.Clone(data),
		}
	}

	// The H.264 group begins only at an IDR with its complete configuration.
	payload, description := s.avc.accessUnit(data, buf.sequence, damaged)
	if len(payload) == 0 {
		return nil, nil
	}
	var header *frametrack.GroupHeader
	if description != nil {
		header = &frametrack.GroupHeader{
			Codec:              s.header.Codec,
			Width:              s.header.Width,
			Height:             s.header.Height,
			DecoderDescription: description,
		}
	}
	return header, &frametrack.Frame{
		CapturedAt: timestamp.New(time.Now()),
		Keyframe:   header != nil,
		Data:       payload,
	}
}

// signal wakes a pending read while the descriptor still belongs to the source.
func (s *Source) signal() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if !s.closed {
		s.wakeReader()
	}
}

// wakeReader posts an event while mtx is held.
func (s *Source) wakeReader() {
	// An eventfd write of one wakes the reader. A full counter is already ready.
	value := [8]byte{}
	binary.NativeEndian.PutUint64(value[:], 1)
	for {
		if _, err := unix.Write(s.wake, value[:]); err != unix.EINTR {
			return
		}
	}
}

// _ is a type assertion.
var _ frametrack.Source = (*Source)(nil)
