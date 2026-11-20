//go:build linux

package v4l2

import (
	"runtime"
	"unsafe"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/frametrack"
	"golang.org/x/sys/unix"
)

const (
	// captureType selects V4L2_BUF_TYPE_VIDEO_CAPTURE.
	captureType = 1
	// mmapMemory selects V4L2_MEMORY_MMAP.
	mmapMemory = 1
	// capCapture identifies single-planar video capture.
	capCapture = 1
	// capStreaming indicates streaming I/O support.
	capStreaming = 0x04000000
	// capDevice indicates that deviceCaps describes this specific node.
	capDevice = 0x80000000
	// bufferError marks a damaged captured buffer.
	bufferError = 0x0040
	// pixelMJPEG is the V4L2 fourcc for motion JPEG.
	pixelMJPEG = 'M' | 'J'<<8 | 'P'<<16 | 'G'<<24
	// ioctlWrite marks a request with input bytes.
	ioctlWrite = 1
	// ioctlRead marks a request with output bytes.
	ioctlRead = 2
	// ioctlReadWrite marks a request that both reads and writes the argument.
	ioctlReadWrite = ioctlRead | ioctlWrite
)

// capabilities mirrors struct v4l2_capability from linux/videodev2.h.
type capabilities struct {
	_ [84]byte
	// all reports capabilities shared by the driver.
	all uint32
	// device reports capabilities of the opened video node.
	device uint32
	_      [12]byte
}

// captureFormat mirrors struct v4l2_format with its single-planar member.
type captureFormat struct {
	// typ selects the buffer type and therefore the union member.
	typ uint32
	_   [0]uintptr
	// width is the negotiated image width.
	width uint32
	// height is the negotiated image height.
	height uint32
	// format is the negotiated pixel format fourcc.
	format uint32
	_      [188]byte
}

// requestBuffers mirrors struct v4l2_requestbuffers.
type requestBuffers struct {
	// count asks for and returns the number of capture buffers.
	count uint32
	// typ selects the capture queue.
	typ uint32
	// memory selects how userspace accesses the buffers.
	memory uint32
	_      [8]byte
}

// captureBuffer mirrors struct v4l2_buffer with its mmap offset member.
type captureBuffer struct {
	// index identifies a buffer within the capture queue.
	index uint32
	// typ selects the capture queue.
	typ uint32
	// used counts bytes available in the mapped buffer.
	used uint32
	// flags describe the captured data and the driver's buffer state.
	flags uint32
	_     uint32
	_     unix.Timeval
	_     [16]byte
	_     uint32
	// memory selects how userspace accesses the buffers.
	memory uint32
	_      [0]uintptr
	// offset identifies the buffer's mmap region in the device.
	offset uint32
	_      [unsafe.Sizeof(uintptr(0)) - 4]byte
	// length is the size of the mapping.
	length uint32
	_      [8]byte
}

// ioctl sends a typed V4L2 argument through the Linux userspace ABI.
func ioctl[T any](fd int, direction, number uintptr, arg *T) error {
	// Linux PowerPC and MIPS place direction bits above a 13-bit size field.
	shift := 30
	switch runtime.GOARCH {
	case "ppc64", "ppc64le", "mips", "mipsle", "mips64", "mips64le":
		shift = 29
		direction = direction&ioctlRead | (direction&ioctlWrite)<<2
	}
	request := direction<<shift | unsafe.Sizeof(*arg)<<16 | 'V'<<8 | number

	// Return the first result other than a signal interruption.
	for {
		_, _, err := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), request, uintptr(unsafe.Pointer(arg)))
		switch err {
		case 0:
			return nil
		case unix.EINTR:
			continue
		default:
			return err
		}
	}
}

// selectFormat selects streaming MJPEG at the device's current size.
func selectFormat(fd int) (*frametrack.GroupHeader, error) {
	// Check the opened node's capture capabilities.
	var caps capabilities
	if err := ioctl(fd, ioctlRead, 0, &caps); err != nil {
		return nil, errors.Wrap(err, "query camera capabilities")
	}
	flags := caps.all
	if flags&capDevice != 0 {
		flags = caps.device
	}
	if flags&(capCapture|capStreaming) != capCapture|capStreaming {
		return nil, errors.New("camera does not support single-planar streaming capture")
	}

	// Preserve the current dimensions and verify the driver accepted MJPEG.
	image := captureFormat{typ: captureType}
	if err := ioctl(fd, ioctlReadWrite, 4, &image); err != nil {
		return nil, errors.Wrap(err, "read camera format")
	}
	if image.format != pixelMJPEG {
		image = captureFormat{typ: captureType, width: image.width, height: image.height, format: pixelMJPEG}
		if err := ioctl(fd, ioctlReadWrite, 5, &image); err != nil {
			return nil, errors.Wrap(err, "select camera format")
		}
		if image.format != pixelMJPEG {
			return nil, errors.New("camera does not offer MJPEG")
		}
	}

	// Publish the size the driver actually negotiated.
	return &frametrack.GroupHeader{
		Codec:  frametrack.Codec_CODEC_JPEG,
		Width:  image.width,
		Height: image.height,
	}, nil
}
