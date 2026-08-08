package s4wave_viewer_native

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func testLaunch() *NativeViewerLaunchRecord {
	return &NativeViewerLaunchRecord{WireVersion: NativeViewerWireVersion, ProtocolVersion: NativeViewerProtocolVersion, LaunchId: "launch:1", SessionObjectKey: "session:1", SpaceObjectKey: "space:1", ManifestObjectKey: "manifest:1", ManifestDigest: "sha256:1", ViewerObjectKey: "viewer:1", ViewerProfile: "default", ResourceScopeSessionObjectKey: "session:1", SelectedStateKey: "state:1", LaunchNonce: "nonce:1", Io: &NativeViewerIODescriptor{InputFd: 0, OutputFd: 1, DiagnosticFd: 2, InputMode: NativeViewerFDMode_NATIVE_VIEWER_FD_MODE_READ, OutputMode: NativeViewerFDMode_NATIVE_VIEWER_FD_MODE_WRITE}, Endpoints: []*NativeViewerEndpointDescriptor{
		{Kind: NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_RECORD, Fd: RecordFD, Transport: NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_LENGTH_DELIMITED_PROTO, ServiceId: "native.viewer.record", ProtocolVersion: NativeViewerProtocolVersion, Required: true, CloseOnExit: true},
		{Kind: NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_READINESS, Fd: ReadinessFD, Transport: NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_LENGTH_DELIMITED_PROTO, ServiceId: "native.viewer.readiness", ProtocolVersion: NativeViewerProtocolVersion, Required: true, CloseOnExit: true},
		{Kind: NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_RESOURCE, Fd: ResourceFD, Transport: NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC, ServiceId: "resource.ResourceService", ProtocolVersion: NativeViewerProtocolVersion, Required: true, CloseOnExit: true},
		{Kind: NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_STATE, Fd: StateFD, Transport: NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC, ServiceId: "s4wave.viewer.native.StateService", ProtocolVersion: NativeViewerProtocolVersion, Required: true, CloseOnExit: true},
		{Kind: NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_CONTROL, Fd: ControlFD, Transport: NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC, ServiceId: "s4wave.viewer.native.ControlService", ProtocolVersion: NativeViewerProtocolVersion, Required: true, CloseOnExit: true},
	}}
}

func TestCanonicalLaunchFixture(t *testing.T) {
	frame, err := os.ReadFile("canonical-launch-frame.bin")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(frame)
	if got := fmt.Sprintf("%x", digest); got != "d18fa1934a448c72d58ef9f31cc14333d97e63a19f2cbc33d2419b1bd8ecd5ca" {
		t.Fatalf("digest=%s", got)
	}
	if _, err := ReadLaunchRecord(bytes.NewReader(frame)); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchValidationAndFraming(t *testing.T) {
	record := testLaunch()
	if err := ValidateLaunchRecord(record); err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := WriteLaunchRecord(&buffer, record); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLaunchRecord(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if !got.EqualVT(record) {
		t.Fatalf("roundtrip=%v", got)
	}
	for name, mutate := range map[string]func(*NativeViewerLaunchRecord){
		"version":         func(v *NativeViewerLaunchRecord) { v.WireVersion++ },
		"scope":           func(v *NativeViewerLaunchRecord) { v.ResourceScopeSessionObjectKey = "session:other" },
		"duplicate fd":    func(v *NativeViewerLaunchRecord) { v.Endpoints[1].Fd = RecordFD },
		"daemon endpoint": func(v *NativeViewerLaunchRecord) { v.Endpoints[0].ServiceId = "spacewave.socket" },
	} {
		t.Run(name, func(t *testing.T) {
			value := record.CloneVT()
			mutate(value)
			if err := ValidateLaunchRecord(value); err == nil {
				t.Fatal("accepted invalid launch")
			}
		})
	}
}

func TestLaunchFramingBoundsAndTrailingRecord(t *testing.T) {
	var oversized bytes.Buffer
	oversized.WriteByte(0xff)
	oversized.WriteByte(0xff)
	oversized.WriteByte(0x07)
	if _, err := ReadLaunchRecord(&oversized); err == nil {
		t.Fatal("accepted oversized frame")
	}
	var buffer bytes.Buffer
	if err := WriteLaunchRecord(&buffer, testLaunch()); err != nil {
		t.Fatal(err)
	}
	if err := WriteLaunchRecord(&buffer, testLaunch()); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLaunchRecord(&buffer); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing err=%v", err)
	}
	if !errors.Is(ValidateLaunchRecord(nil), ErrInvalidLaunch) {
		t.Fatal("nil validation not typed")
	}
}

func TestReadinessEchoValidationAndFraming(t *testing.T) {
	launch := testLaunch()
	readiness := &NativeViewerReadinessRecord{WireVersion: NativeViewerWireVersion, ProtocolVersion: NativeViewerProtocolVersion, LaunchId: launch.LaunchId, SessionObjectKey: launch.SessionObjectKey, SpaceObjectKey: launch.SpaceObjectKey, ManifestObjectKey: launch.ManifestObjectKey, ManifestDigest: launch.ManifestDigest, ViewerObjectKey: launch.ViewerObjectKey, ViewerProfile: launch.ViewerProfile, ResourceScopeSessionObjectKey: launch.ResourceScopeSessionObjectKey, SelectedStateKey: launch.SelectedStateKey, Status: NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_READY, ResourceRevision: 1, ResourceCursor: 1, FrameSequence: 1}
	var buffer bytes.Buffer
	if err := WriteReadinessRecord(&buffer, readiness, launch); err != nil {
		t.Fatal(err)
	}
	got, err := ReadReadinessRecord(&buffer, launch)
	if err != nil || !got.EqualVT(readiness) {
		t.Fatalf("readiness=%v err=%v", got, err)
	}
	stale := readiness.CloneVT()
	stale.SessionObjectKey = "session:other"
	if err := ValidateReadinessRecord(stale, launch); err == nil {
		t.Fatal("accepted stale readiness")
	}
}

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	return 1, nil
}

type shortReader struct{ data []byte }

func (r *shortReader) Read(data []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	data[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}

func TestLaunchValidationEveryDescriptorAndIdentityField(t *testing.T) {
	fields := []struct {
		name   string
		mutate func(*NativeViewerLaunchRecord)
	}{
		{"launch", func(v *NativeViewerLaunchRecord) { v.LaunchId = "\n" }}, {"session", func(v *NativeViewerLaunchRecord) { v.SessionObjectKey = "" }}, {"space", func(v *NativeViewerLaunchRecord) { v.SpaceObjectKey = "\xff" }}, {"manifest", func(v *NativeViewerLaunchRecord) { v.ManifestObjectKey = "\x1b" }}, {"digest", func(v *NativeViewerLaunchRecord) { v.ManifestDigest = "" }}, {"viewer", func(v *NativeViewerLaunchRecord) { v.ViewerObjectKey = "\t" }}, {"profile", func(v *NativeViewerLaunchRecord) { v.ViewerProfile = "\n" }}, {"state", func(v *NativeViewerLaunchRecord) { v.SelectedStateKey = "" }}, {"nonce", func(v *NativeViewerLaunchRecord) { v.LaunchNonce = "\u009f" }},
		{"input mode", func(v *NativeViewerLaunchRecord) {
			v.Io.InputMode = NativeViewerFDMode_NATIVE_VIEWER_FD_MODE_WRITE
		}}, {"output mode", func(v *NativeViewerLaunchRecord) {
			v.Io.OutputMode = NativeViewerFDMode_NATIVE_VIEWER_FD_MODE_READ
		}}, {"diagnostic fd", func(v *NativeViewerLaunchRecord) { v.Io.DiagnosticFd = 9 }}, {"close", func(v *NativeViewerLaunchRecord) { v.Endpoints[0].CloseOnExit = false }}, {"required", func(v *NativeViewerLaunchRecord) { v.Endpoints[0].Required = false }}, {"transport", func(v *NativeViewerLaunchRecord) {
			v.Endpoints[0].Transport = NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC
		}}, {"service", func(v *NativeViewerLaunchRecord) { v.Endpoints[0].ServiceId = "native.viewer.record.alias" }}, {"kind", func(v *NativeViewerLaunchRecord) {
			v.Endpoints[0].Kind = NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_CONTROL
		}}, {"protocol", func(v *NativeViewerLaunchRecord) { v.Endpoints[0].ProtocolVersion++ }}, {"unknown fd", func(v *NativeViewerLaunchRecord) { v.Endpoints[0].Fd = 8 }},
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			value := testLaunch()
			field.mutate(value)
			if err := ValidateLaunchRecord(value); err == nil {
				t.Fatal("accepted invalid field")
			}
		})
	}
	value := testLaunch()
	value.Endpoints = append(value.Endpoints, value.Endpoints[0].CloneVT())
	if err := ValidateLaunchRecord(value); err == nil {
		t.Fatal("accepted endpoint alias")
	}
}

func TestLaunchShortIOAndMalformedReadiness(t *testing.T) {
	if err := WriteLaunchRecord(shortWriter{}, testLaunch()); err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := WriteLaunchRecord(&buffer, testLaunch()); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLaunchRecord(&shortReader{data: buffer.Bytes()}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReadinessRecord(bytes.NewBuffer([]byte{0x80}), testLaunch()); err == nil {
		t.Fatal("accepted malformed varint")
	}
	readiness := &NativeViewerReadinessRecord{WireVersion: NativeViewerWireVersion, ProtocolVersion: NativeViewerProtocolVersion, LaunchId: "launch:1", SessionObjectKey: "session:1", SpaceObjectKey: "space:1", ManifestObjectKey: "manifest:1", ManifestDigest: "sha256:1", ViewerObjectKey: "viewer:1", ViewerProfile: "default", ResourceScopeSessionObjectKey: "session:1", SelectedStateKey: "state:1", Status: NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_FAILED, Detail: "failed", TerminalRestoreAttempted: true, AllWorkersJoined: true}
	var ready bytes.Buffer
	if err := WriteReadinessRecord(&ready, readiness, testLaunch()); err != nil {
		t.Fatal(err)
	}
	ready.WriteByte(0)
	if _, err := ReadReadinessRecord(&ready, testLaunch()); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing readiness err=%v", err)
	}
}

func TestReadinessValidationStatusMatrix(t *testing.T) {
	launch := testLaunch()
	base := &NativeViewerReadinessRecord{WireVersion: NativeViewerWireVersion, ProtocolVersion: NativeViewerProtocolVersion, LaunchId: launch.LaunchId, SessionObjectKey: launch.SessionObjectKey, SpaceObjectKey: launch.SpaceObjectKey, ManifestObjectKey: launch.ManifestObjectKey, ManifestDigest: launch.ManifestDigest, ViewerObjectKey: launch.ViewerObjectKey, ViewerProfile: launch.ViewerProfile, ResourceScopeSessionObjectKey: launch.ResourceScopeSessionObjectKey, SelectedStateKey: launch.SelectedStateKey}
	ready := base.CloneVT()
	ready.Status = NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_READY
	ready.ResourceRevision, ready.ResourceCursor, ready.FrameSequence = 1, 1, 1
	if err := ValidateReadinessRecord(ready, launch); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*NativeViewerReadinessRecord){
		"ready frame zero":    func(v *NativeViewerReadinessRecord) { v.FrameSequence = 0 },
		"ready revision zero": func(v *NativeViewerReadinessRecord) { v.ResourceRevision = 0 },
		"ready cursor zero":   func(v *NativeViewerReadinessRecord) { v.ResourceCursor = 0 },
		"ready detail":        func(v *NativeViewerReadinessRecord) { v.Detail = "detail" },
		"ready cleanup":       func(v *NativeViewerReadinessRecord) { v.AllWorkersJoined = true },
		"profile echo":        func(v *NativeViewerReadinessRecord) { v.ViewerProfile = "other" },
		"scope echo":          func(v *NativeViewerReadinessRecord) { v.ResourceScopeSessionObjectKey = "session:other" },
		"state echo":          func(v *NativeViewerReadinessRecord) { v.SelectedStateKey = "state:other" },
	} {
		t.Run(name, func(t *testing.T) {
			value := ready.CloneVT()
			mutate(value)
			if err := ValidateReadinessRecord(value, launch); err == nil {
				t.Fatal("accepted invalid readiness")
			}
		})
	}
	for _, status := range []NativeViewerReadinessStatus{NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_FAILED, NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_CANCELLED} {
		value := base.CloneVT()
		value.Status = status
		value.Detail = "failed"
		value.TerminalRestoreAttempted = true
		value.AllWorkersJoined = true
		if err := ValidateReadinessRecord(value, launch); err != nil {
			t.Fatal(err)
		}
		for name, mutate := range map[string]func(*NativeViewerReadinessRecord){"empty detail": func(v *NativeViewerReadinessRecord) { v.Detail = "" }, "unsafe detail": func(v *NativeViewerReadinessRecord) { v.Detail = "bad\n" }, "missing restore": func(v *NativeViewerReadinessRecord) { v.TerminalRestoreAttempted = false }, "missing join": func(v *NativeViewerReadinessRecord) { v.AllWorkersJoined = false }} {
			t.Run(string(status)+"/"+name, func(t *testing.T) {
				bad := value.CloneVT()
				mutate(bad)
				if err := ValidateReadinessRecord(bad, launch); err == nil {
					t.Fatal("accepted invalid terminal readiness")
				}
			})
		}
	}
}

func TestReadReadinessRecordLiveDoesNotWaitForEOF(t *testing.T) {
	launch := testLaunch()
	readiness := &NativeViewerReadinessRecord{WireVersion: NativeViewerWireVersion, ProtocolVersion: NativeViewerProtocolVersion, LaunchId: launch.LaunchId, SessionObjectKey: launch.SessionObjectKey, SpaceObjectKey: launch.SpaceObjectKey, ManifestObjectKey: launch.ManifestObjectKey, ManifestDigest: launch.ManifestDigest, ViewerObjectKey: launch.ViewerObjectKey, ViewerProfile: launch.ViewerProfile, ResourceScopeSessionObjectKey: launch.ResourceScopeSessionObjectKey, SelectedStateKey: launch.SelectedStateKey, Status: NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_READY, FrameSequence: 1, ResourceRevision: 1, ResourceCursor: 1}
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() { _, err := ReadReadinessRecordLive(pr, launch); done <- err }()
	if err := WriteReadinessRecord(pw, readiness, launch); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("live readiness read waited for EOF")
	}
	_ = pw.Close()
	_ = pr.Close()
}
