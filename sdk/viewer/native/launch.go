// Package native owns the bounded inherited native-viewer launch record.
package s4wave_viewer_native

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"unicode"
	"unicode/utf8"

	pkgerrors "github.com/pkg/errors"
)

const (
	NativeViewerWireVersion     uint32 = 1
	NativeViewerProtocolVersion uint32 = 1
	MaxRecordBytes                     = 64 * 1024
	RecordFD                           = 3
	ReadinessFD                        = 4
	ResourceFD                         = 5
	StateFD                            = 6
	ControlFD                          = 7
)

var (
	ErrInvalidLaunch    = errors.New("nativeviewer: invalid launch record")
	ErrInvalidReadiness = errors.New("nativeviewer: invalid readiness record")
	ErrFrameTooLarge    = errors.New("nativeviewer: frame exceeds bound")
	ErrTrailingFrame    = errors.New("nativeviewer: trailing frame")
)

func validSafeText(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validIdentity(value string) bool { return validSafeText(value, 1000) }

// ValidateLaunchRecord validates the frozen identity and fixed inherited FD contract.
func ValidateLaunchRecord(record *NativeViewerLaunchRecord) error {
	if record == nil || record.GetWireVersion() != NativeViewerWireVersion || record.GetProtocolVersion() != NativeViewerProtocolVersion {
		return ErrInvalidLaunch
	}
	for _, value := range []string{record.GetLaunchId(), record.GetSessionObjectKey(), record.GetSpaceObjectKey(), record.GetManifestObjectKey(), record.GetManifestDigest(), record.GetViewerObjectKey(), record.GetViewerProfile(), record.GetResourceScopeSessionObjectKey(), record.GetSelectedStateKey(), record.GetLaunchNonce()} {
		if !validIdentity(value) {
			return ErrInvalidLaunch
		}
	}
	if record.GetResourceScopeSessionObjectKey() != record.GetSessionObjectKey() {
		return pkgerrors.Wrap(ErrInvalidLaunch, "resource scope")
	}
	ioDesc := record.GetIo()
	if ioDesc == nil || ioDesc.GetInputFd() != 0 || ioDesc.GetOutputFd() != 1 || ioDesc.GetDiagnosticFd() != 2 || ioDesc.GetInputMode() != NativeViewerFDMode_NATIVE_VIEWER_FD_MODE_READ || ioDesc.GetOutputMode() != NativeViewerFDMode_NATIVE_VIEWER_FD_MODE_WRITE {
		return pkgerrors.Wrap(ErrInvalidLaunch, "IO descriptors")
	}
	expected := map[int32]struct {
		kind      NativeViewerEndpointKind
		transport NativeViewerTransport
		service   string
	}{
		RecordFD:    {NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_RECORD, NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_LENGTH_DELIMITED_PROTO, "native.viewer.record"},
		ReadinessFD: {NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_READINESS, NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_LENGTH_DELIMITED_PROTO, "native.viewer.readiness"},
		ResourceFD:  {NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_RESOURCE, NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC, "resource.ResourceService"},
		StateFD:     {NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_STATE, NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC, SRPCStateServiceServiceID},
		ControlFD:   {NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_CONTROL, NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC, SRPCControlServiceServiceID},
	}
	if len(record.GetEndpoints()) != len(expected) {
		return pkgerrors.Wrap(ErrInvalidLaunch, "endpoint count")
	}
	seen := map[int32]bool{}
	for _, endpoint := range record.GetEndpoints() {
		if endpoint == nil || seen[endpoint.GetFd()] || endpoint.GetFd() < RecordFD || endpoint.GetFd() > ControlFD {
			return pkgerrors.Wrap(ErrInvalidLaunch, "endpoint alias")
		}
		expect, ok := expected[endpoint.GetFd()]
		if !ok || endpoint.GetKind() != expect.kind || endpoint.GetTransport() != expect.transport || endpoint.GetServiceId() != expect.service || endpoint.GetProtocolVersion() != NativeViewerProtocolVersion || !endpoint.GetRequired() || !endpoint.GetCloseOnExit() {
			return pkgerrors.Wrap(ErrInvalidLaunch, "endpoint descriptor")
		}
		seen[endpoint.GetFd()] = true
	}
	return nil
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	length, err := binary.ReadUvarint(reader)
	if err != nil {
		return nil, err
	}
	if length == 0 || length > MaxRecordBytes {
		return nil, pkgerrors.Wrapf(ErrFrameTooLarge, "%d", length)
	}
	frame := make([]byte, int(length))
	if _, err := io.ReadFull(reader, frame); err != nil {
		return nil, err
	}
	return frame, nil
}
func writeFrame(writer io.Writer, frame []byte) error {
	if len(frame) == 0 || len(frame) > MaxRecordBytes {
		return pkgerrors.Wrapf(ErrFrameTooLarge, "%d", len(frame))
	}
	var prefix [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(prefix[:], uint64(len(frame)))
	if err := writeFull(writer, prefix[:n]); err != nil {
		return err
	}
	return writeFull(writer, frame)
}
func writeFull(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if n < 0 || n > len(data) {
			return errors.New("nativeviewer: invalid write count")
		}
		data = data[n:]
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

// ReadLaunchRecord reads one bounded length-delimited protobuf and rejects trailing records.
func ReadLaunchRecord(reader io.Reader) (*NativeViewerLaunchRecord, error) {
	buffered := bufio.NewReader(reader)
	frame, err := readFrame(buffered)
	if err != nil {
		return nil, err
	}
	record := &NativeViewerLaunchRecord{}
	if err := record.UnmarshalVT(frame); err != nil {
		return nil, err
	}
	if err := ValidateLaunchRecord(record); err != nil {
		return nil, err
	}
	if _, err := buffered.Peek(1); err == nil {
		return nil, errors.New("nativeviewer: trailing launch record")
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}
	return record, nil
}

// ValidateReadinessRecord validates readiness semantics and exact launch identity echoes.
func ValidateReadinessRecord(readiness *NativeViewerReadinessRecord, launch *NativeViewerLaunchRecord) error {
	if readiness == nil || readiness.GetWireVersion() != NativeViewerWireVersion || readiness.GetProtocolVersion() != NativeViewerProtocolVersion || !validIdentity(readiness.GetLaunchId()) || !validIdentity(readiness.GetSessionObjectKey()) || !validIdentity(readiness.GetSpaceObjectKey()) || !validIdentity(readiness.GetManifestObjectKey()) || !validIdentity(readiness.GetManifestDigest()) || !validIdentity(readiness.GetViewerObjectKey()) || !validIdentity(readiness.GetViewerProfile()) || !validIdentity(readiness.GetResourceScopeSessionObjectKey()) || !validIdentity(readiness.GetSelectedStateKey()) {
		return ErrInvalidReadiness
	}
	if launch != nil && (readiness.GetLaunchId() != launch.GetLaunchId() || readiness.GetSessionObjectKey() != launch.GetSessionObjectKey() || readiness.GetSpaceObjectKey() != launch.GetSpaceObjectKey() || readiness.GetManifestObjectKey() != launch.GetManifestObjectKey() || readiness.GetManifestDigest() != launch.GetManifestDigest() || readiness.GetViewerObjectKey() != launch.GetViewerObjectKey() || readiness.GetViewerProfile() != launch.GetViewerProfile() || readiness.GetResourceScopeSessionObjectKey() != launch.GetResourceScopeSessionObjectKey() || readiness.GetSelectedStateKey() != launch.GetSelectedStateKey()) {
		return pkgerrors.Wrap(ErrInvalidReadiness, "readiness identity echo")
	}
	if readiness.GetStatus() == NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_READY {
		if readiness.GetFrameSequence() == 0 || readiness.GetResourceRevision() == 0 || readiness.GetResourceCursor() == 0 || readiness.GetTerminalRestoreAttempted() || readiness.GetAllWorkersJoined() || readiness.GetDetail() != "" {
			return pkgerrors.Wrap(ErrInvalidReadiness, "READY semantics")
		}
		return nil
	}
	if readiness.GetStatus() != NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_FAILED && readiness.GetStatus() != NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_CANCELLED {
		return pkgerrors.Wrap(ErrInvalidReadiness, "readiness status")
	}
	if !validSafeText(readiness.GetDetail(), 4096) || !readiness.GetTerminalRestoreAttempted() || !readiness.GetAllWorkersJoined() {
		return pkgerrors.Wrap(ErrInvalidReadiness, "terminal readiness semantics")
	}
	return nil
}

// ReadReadinessRecord reads one bounded readiness protobuf and verifies the launch echo.
func ReadReadinessRecord(reader io.Reader, launch *NativeViewerLaunchRecord) (*NativeViewerReadinessRecord, error) {
	buffered := bufio.NewReader(reader)
	frame, err := readFrame(buffered)
	if err != nil {
		return nil, err
	}
	readiness := &NativeViewerReadinessRecord{}
	if err := readiness.UnmarshalVT(frame); err != nil {
		return nil, err
	}
	if err := ValidateReadinessRecord(readiness, launch); err != nil {
		return nil, err
	}
	if _, err := buffered.Peek(1); err == nil {
		return nil, ErrTrailingFrame
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}
	return readiness, nil
}

// WriteReadinessRecord writes one validated bounded length-delimited readiness record.
func WriteReadinessRecord(writer io.Writer, readiness *NativeViewerReadinessRecord, launch *NativeViewerLaunchRecord) error {
	if err := ValidateReadinessRecord(readiness, launch); err != nil {
		return err
	}
	frame, err := readiness.MarshalVT()
	if err != nil {
		return err
	}
	return writeFrame(writer, frame)
}

// WriteLaunchRecord writes one validated bounded length-delimited protobuf.
func WriteLaunchRecord(writer io.Writer, record *NativeViewerLaunchRecord) error {
	if err := ValidateLaunchRecord(record); err != nil {
		return err
	}
	frame, err := record.MarshalVT()
	if err != nil {
		return err
	}
	return writeFrame(writer, frame)
}
