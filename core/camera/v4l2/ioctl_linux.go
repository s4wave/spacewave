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
	// pixelH264 is the V4L2 fourcc for an H.264 access unit with start codes.
	pixelH264 = 'H' | '2'<<8 | '6'<<16 | '4'<<24
	// forceKeyFrameControl is V4L2_CID_MPEG_VIDEO_FORCE_KEY_FRAME.
	forceKeyFrameControl = 0x00990900 + 229
	// ioctlWrite marks a request with input bytes.
	ioctlWrite = 1
	// ioctlRead marks a request with output bytes.
	ioctlRead = 2
	// ioctlReadWrite marks a request that both reads and writes the argument.
	ioctlReadWrite = ioctlRead | ioctlWrite
)

// V4L2 request numbers come from linux/videodev2.h. ioctl adds the direction
// and the argument's size, as the kernel's _IOC macro does.
const (
	vidiocQuerycap  = 0
	vidiocEnumFmt   = 2
	vidiocGFmt      = 4
	vidiocSFmt      = 5
	vidiocReqbufs   = 8
	vidiocQuerybuf  = 9
	vidiocQbuf      = 15
	vidiocDqbuf     = 17
	vidiocStreamon  = 18
	vidiocStreamoff = 19
	vidiocSCtrl     = 28
	vidiocQueryctrl = 36
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
	// sequence counts captured buffers, including those lost to the reader.
	sequence uint32
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

// formatDescription mirrors struct v4l2_fmtdesc.
type formatDescription struct {
	// index selects a format in the capture queue.
	index uint32
	// typ selects the capture queue.
	typ uint32
	_   [36]byte
	// format identifies a supported pixel format by its fourcc.
	format uint32
	_      [16]byte
}

// queryControl mirrors struct v4l2_queryctrl.
type queryControl struct {
	// id identifies the control to query.
	id uint32
	_  [52]byte
	// flags describe whether the driver enables writes to this control.
	flags uint32
	_     [8]byte
}

// setControl mirrors struct v4l2_control for a button press.
type setControl struct {
	// id identifies the control to activate.
	id uint32
	_  int32
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

// selectFormat selects the preferred compressed capture format at the device's current size.
func selectFormat(fd int) (*frametrack.GroupHeader, error) {
	// Check the opened node's capture capabilities.
	var caps capabilities
	if err := ioctl(fd, ioctlRead, vidiocQuerycap, &caps); err != nil {
		return nil, errors.Wrap(err, "query camera capabilities")
	}
	flags := caps.all
	if flags&capDevice != 0 {
		flags = caps.device
	}
	if flags&(capCapture|capStreaming) != capCapture|capStreaming {
		return nil, errors.New("camera does not support single-planar streaming capture")
	}

	// Preserve the current dimensions and verify the driver accepted the format.
	format, codec, err := selectCodec(fd)
	if err != nil {
		return nil, err
	}
	image := captureFormat{typ: captureType}
	if err := ioctl(fd, ioctlReadWrite, vidiocGFmt, &image); err != nil {
		return nil, errors.Wrap(err, "read camera format")
	}
	if image.format != format {
		image = captureFormat{typ: captureType, width: image.width, height: image.height, format: format}
		if err := ioctl(fd, ioctlReadWrite, vidiocSFmt, &image); err != nil {
			return nil, errors.Wrap(err, "select camera format")
		}
		if image.format != format {
			return nil, errors.New("camera changed the requested compressed format")
		}
	}

	// Publish the size the driver actually negotiated.
	return &frametrack.GroupHeader{
		Codec:  codec,
		Width:  image.width,
		Height: image.height,
	}, nil
}

// selectCodec prefers H.264 among the capture queue's advertised formats.
func selectCodec(fd int) (uint32, frametrack.Codec, error) {
	// V4L2 uses EINVAL to mark the end of the format list.
	hasMJPEG := false
	for index := uint32(0); ; index++ {
		desc := formatDescription{index: index, typ: captureType}
		err := ioctl(fd, ioctlReadWrite, vidiocEnumFmt, &desc)
		if err == unix.EINVAL {
			break
		}
		if err != nil {
			return 0, 0, errors.Wrap(err, "enumerate camera formats")
		}

		// Remember MJPEG if the camera advertises it before H.264.
		switch desc.format {
		case pixelH264:
			return pixelH264, frametrack.Codec_CODEC_H264_AVC, nil
		case pixelMJPEG:
			hasMJPEG = true
		}
	}

	// A camera without native compression cannot provide a pass-through track.
	if hasMJPEG {
		return pixelMJPEG, frametrack.Codec_CODEC_JPEG, nil
	}
	return 0, 0, errors.New("camera offers neither H.264 nor MJPEG")
}

// canForceKeyframe reports whether the camera enables the keyframe button.
func canForceKeyframe(fd int) bool {
	ctrl := queryControl{id: forceKeyFrameControl}
	const unavailable = 0x0001 | 0x0004 | 0x0010 // disabled, read-only, inactive
	return ioctl(fd, ioctlReadWrite, vidiocQueryctrl, &ctrl) == nil && ctrl.flags&unavailable == 0
}
