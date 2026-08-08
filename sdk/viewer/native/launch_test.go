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

// testEndpoint returns one required child-owned descriptor for the current protocol.
func testEndpoint(kind NativeViewerEndpointKind, fd int32, transport NativeViewerTransport, service string) *NativeViewerEndpointDescriptor {
	return &NativeViewerEndpointDescriptor{
		Kind:            kind,
		Fd:              fd,
		Transport:       transport,
		ServiceId:       service,
		ProtocolVersion: NativeViewerProtocolVersion,
		Required:        true,
		CloseOnExit:     true,
	}
}

// testLaunch returns a valid launch record with every fixed endpoint descriptor.
func testLaunch() *NativeViewerLaunchRecord {
	return &NativeViewerLaunchRecord{
		WireVersion:     NativeViewerWireVersion,
		ProtocolVersion: NativeViewerProtocolVersion,
		LaunchId:        "launch:1",

		LlmSessionObjectKey:              "session:1",
		SpaceObjectKey:                   "space:1",
		ResourceScopeLlmSessionObjectKey: "session:1",
		SelectedStateKey:                 "state:1",
		SpacewaveSessionRef: &NativeViewerSpacewaveSessionRef{
			ProviderResourceId: "resource-1",
			ProviderId:         "provider-1",
			ProviderAccountId:  "account-1",
		},

		ManifestObjectKey: "manifest:1",
		ManifestDigest:    "sha256:1",
		ViewerObjectKey:   "viewer:1",
		ViewerProfile:     "default",
		LaunchNonce:       "nonce:1",

		Io: &NativeViewerIODescriptor{
			InputFd:      0,
			OutputFd:     1,
			DiagnosticFd: 2,
			InputMode:    NativeViewerFDMode_NATIVE_VIEWER_FD_MODE_READ,
			OutputMode:   NativeViewerFDMode_NATIVE_VIEWER_FD_MODE_WRITE,
		},
		Endpoints: []*NativeViewerEndpointDescriptor{
			testEndpoint(NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_RECORD, RecordFD, NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_LENGTH_DELIMITED_PROTO, "native.viewer.record"),
			testEndpoint(NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_READINESS, ReadinessFD, NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_LENGTH_DELIMITED_PROTO, "native.viewer.readiness"),
			testEndpoint(NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_RESOURCE, ResourceFD, NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC, "resource.ResourceService"),
			testEndpoint(NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_STATE, StateFD, NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC, SRPCStateServiceServiceID),
			testEndpoint(NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_CONTROL, ControlFD, NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC, SRPCControlServiceServiceID),
		},
	}
}

// testReadiness returns readiness identity that exactly echoes one valid launch.
func testReadiness(launch *NativeViewerLaunchRecord) *NativeViewerReadinessRecord {
	return &NativeViewerReadinessRecord{
		WireVersion:     NativeViewerWireVersion,
		ProtocolVersion: NativeViewerProtocolVersion,
		LaunchId:        launch.GetLaunchId(),
		LaunchNonce:     launch.GetLaunchNonce(),

		LlmSessionObjectKey:              launch.GetLlmSessionObjectKey(),
		SpaceObjectKey:                   launch.GetSpaceObjectKey(),
		ResourceScopeLlmSessionObjectKey: launch.GetResourceScopeLlmSessionObjectKey(),
		SelectedStateKey:                 launch.GetSelectedStateKey(),
		SpacewaveSessionRef:              launch.GetSpacewaveSessionRef().CloneVT(),

		ManifestObjectKey: launch.GetManifestObjectKey(),
		ManifestDigest:    launch.GetManifestDigest(),
		ViewerObjectKey:   launch.GetViewerObjectKey(),
		ViewerProfile:     launch.GetViewerProfile(),
	}
}

// TestCanonicalLaunchFixture pins the encoded launch record bytes and digest.
func TestCanonicalLaunchFixture(t *testing.T) {
	frame, err := os.ReadFile("canonical-launch-frame.bin")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(frame)
	if got := fmt.Sprintf("%x", digest); got != "9984534549e9fd0b4e3bbddb93cd693e8ce9417d718159ddb47d3214d4b51c53" {
		t.Fatalf("digest=%s", got)
	}
	if _, err := ReadLaunchRecord(bytes.NewReader(frame)); err != nil {
		t.Fatal(err)
	}
}

// TestLaunchValidationAndFraming proves a valid record round-trips and identity mutations are rejected.
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
		"scope":           func(v *NativeViewerLaunchRecord) { v.ResourceScopeLlmSessionObjectKey = "session:other" },
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

// TestLaunchFramingBoundsAndTrailingRecord proves empty, oversized, truncated, and concatenated frames are rejected.
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

// TestReadinessEchoValidationAndFraming proves readiness must echo the complete frozen launch identity.
func TestReadinessEchoValidationAndFraming(t *testing.T) {
	launch := testLaunch()
	readiness := testReadiness(launch)
	readiness.Status = NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_READY
	readiness.ResourceRevision, readiness.ResourceCursor, readiness.FrameSequence = 1, 1, 1
	var buffer bytes.Buffer
	if err := WriteReadinessRecord(&buffer, readiness, launch); err != nil {
		t.Fatal(err)
	}
	got, err := ReadReadinessRecord(&buffer, launch)
	if err != nil || !got.EqualVT(readiness) {
		t.Fatalf("readiness=%v err=%v", got, err)
	}
	stale := readiness.CloneVT()
	stale.LlmSessionObjectKey = "session:other"
	if err := ValidateReadinessRecord(stale, launch); err == nil {
		t.Fatal("accepted stale readiness")
	}
}

// shortWriter simulates writer progress without completing a frame.
type shortWriter struct{}

// Write reports a short write after accepting one byte.
func (shortWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	return 1, nil
}

// shortReader returns fixture bytes in deliberately short reads.
type shortReader struct{ data []byte }

// Read returns fixture bytes in bounded chunks.
func (r *shortReader) Read(data []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	data[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}

// TestLaunchValidationEveryDescriptorAndIdentityField proves every inherited descriptor and required identity is validated.
func TestLaunchValidationEveryDescriptorAndIdentityField(t *testing.T) {
	fields := []struct {
		name   string
		mutate func(*NativeViewerLaunchRecord)
	}{
		{"launch", func(v *NativeViewerLaunchRecord) { v.LaunchId = "\n" }},
		{"session", func(v *NativeViewerLaunchRecord) { v.LlmSessionObjectKey = "" }},
		{"space", func(v *NativeViewerLaunchRecord) { v.SpaceObjectKey = "\xff" }},
		{"manifest", func(v *NativeViewerLaunchRecord) { v.ManifestObjectKey = "\x1b" }},
		{"digest", func(v *NativeViewerLaunchRecord) { v.ManifestDigest = "" }},
		{"viewer", func(v *NativeViewerLaunchRecord) { v.ViewerObjectKey = "\t" }},
		{"profile", func(v *NativeViewerLaunchRecord) { v.ViewerProfile = "\n" }},
		{"state", func(v *NativeViewerLaunchRecord) { v.SelectedStateKey = "" }},
		{"nonce", func(v *NativeViewerLaunchRecord) { v.LaunchNonce = "\u009f" }},
		{"protocol version", func(v *NativeViewerLaunchRecord) { v.ProtocolVersion++ }},
		{"legacy protocol version", func(v *NativeViewerLaunchRecord) { v.ProtocolVersion = 1 }},
		{"missing Spacewave SessionRef", func(v *NativeViewerLaunchRecord) { v.SpacewaveSessionRef = nil }},
		{"invalid provider resource DNS label", func(v *NativeViewerLaunchRecord) { v.SpacewaveSessionRef.ProviderResourceId = "Resource" }},
		{"invalid provider DNS label", func(v *NativeViewerLaunchRecord) { v.SpacewaveSessionRef.ProviderId = "provider_1" }},
		{"invalid provider account DNS label", func(v *NativeViewerLaunchRecord) { v.SpacewaveSessionRef.ProviderAccountId = "account-" }},
		{"input mode", func(v *NativeViewerLaunchRecord) {
			v.Io.InputMode = NativeViewerFDMode_NATIVE_VIEWER_FD_MODE_WRITE
		}},
		{"output mode", func(v *NativeViewerLaunchRecord) {
			v.Io.OutputMode = NativeViewerFDMode_NATIVE_VIEWER_FD_MODE_READ
		}},
		{"diagnostic fd", func(v *NativeViewerLaunchRecord) { v.Io.DiagnosticFd = 9 }},
		{"close", func(v *NativeViewerLaunchRecord) { v.Endpoints[0].CloseOnExit = false }},
		{"required", func(v *NativeViewerLaunchRecord) { v.Endpoints[0].Required = false }},
		{"transport", func(v *NativeViewerLaunchRecord) {
			v.Endpoints[0].Transport = NativeViewerTransport_NATIVE_VIEWER_TRANSPORT_SRPC
		}},
		{"service", func(v *NativeViewerLaunchRecord) { v.Endpoints[0].ServiceId = "native.viewer.record.alias" }},
		{"kind", func(v *NativeViewerLaunchRecord) {
			v.Endpoints[0].Kind = NativeViewerEndpointKind_NATIVE_VIEWER_ENDPOINT_KIND_CONTROL
		}},
		{"protocol", func(v *NativeViewerLaunchRecord) { v.Endpoints[0].ProtocolVersion++ }},
		{"unknown fd", func(v *NativeViewerLaunchRecord) { v.Endpoints[0].Fd = 8 }},
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

// TestValidateSpacewaveSessionRef pins every member of the provider tuple to DNS-label syntax and capacity.
func TestValidateSpacewaveSessionRef(t *testing.T) {
	valid := testLaunch().GetSpacewaveSessionRef()
	if err := ValidateSpacewaveSessionRef(valid); err != nil {
		t.Fatal(err)
	}

	long := strings.Repeat("a", 64)
	cases := []struct {
		name   string
		mutate func(*NativeViewerSpacewaveSessionRef)
	}{
		{name: "empty provider resource", mutate: func(ref *NativeViewerSpacewaveSessionRef) { ref.ProviderResourceId = "" }},
		{name: "long provider resource", mutate: func(ref *NativeViewerSpacewaveSessionRef) { ref.ProviderResourceId = long }},
		{name: "controlled provider resource", mutate: func(ref *NativeViewerSpacewaveSessionRef) { ref.ProviderResourceId = "resource\n" }},
		{name: "empty provider", mutate: func(ref *NativeViewerSpacewaveSessionRef) { ref.ProviderId = "" }},
		{name: "long provider", mutate: func(ref *NativeViewerSpacewaveSessionRef) { ref.ProviderId = long }},
		{name: "controlled provider", mutate: func(ref *NativeViewerSpacewaveSessionRef) { ref.ProviderId = "provider\n" }},
		{name: "empty provider account", mutate: func(ref *NativeViewerSpacewaveSessionRef) { ref.ProviderAccountId = "" }},
		{name: "long provider account", mutate: func(ref *NativeViewerSpacewaveSessionRef) { ref.ProviderAccountId = long }},
		{name: "controlled provider account", mutate: func(ref *NativeViewerSpacewaveSessionRef) { ref.ProviderAccountId = "account\n" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref := valid.CloneVT()
			tc.mutate(ref)
			if err := ValidateSpacewaveSessionRef(ref); err == nil {
				t.Fatal("accepted invalid Spacewave SessionRef")
			}
		})
	}
	if err := ValidateSpacewaveSessionRef(nil); err == nil {
		t.Fatal("accepted missing Spacewave SessionRef")
	}
}

// TestLaunchShortIOAndMalformedReadiness proves short I/O, malformed varints, and trailing readiness bytes fail safely.
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
	launch := testLaunch()
	readiness := testReadiness(launch)
	readiness.Status = NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_FAILED
	readiness.Detail = "failed"
	readiness.TerminalRestoreAttempted, readiness.AllWorkersJoined = true, true
	var ready bytes.Buffer
	if err := WriteReadinessRecord(&ready, readiness, launch); err != nil {
		t.Fatal(err)
	}
	ready.WriteByte(0)
	if _, err := ReadReadinessRecord(&ready, launch); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing readiness err=%v", err)
	}
}

// TestReadinessValidationStatusMatrix proves READY and terminal statuses require their distinct evidence fields.
func TestReadinessValidationStatusMatrix(t *testing.T) {
	launch := testLaunch()
	base := testReadiness(launch)
	if err := ValidateReadinessRecord(base, launch); err == nil {
		t.Fatal("accepted unknown readiness status")
	}
	ready := base.CloneVT()
	ready.Status = NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_READY
	ready.ResourceRevision, ready.ResourceCursor, ready.FrameSequence = 1, 1, 1
	if err := ValidateReadinessRecord(ready, launch); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReadinessRecord(ready, nil); err == nil {
		t.Fatal("accepted readiness without launch")
	}
	for name, mutate := range map[string]func(*NativeViewerReadinessRecord){
		"ready frame zero":    func(v *NativeViewerReadinessRecord) { v.FrameSequence = 0 },
		"ready revision zero": func(v *NativeViewerReadinessRecord) { v.ResourceRevision = 0 },
		"ready cursor zero":   func(v *NativeViewerReadinessRecord) { v.ResourceCursor = 0 },
		"ready detail":        func(v *NativeViewerReadinessRecord) { v.Detail = "detail" },
		"ready cleanup":       func(v *NativeViewerReadinessRecord) { v.AllWorkersJoined = true },
		"launch echo":         func(v *NativeViewerReadinessRecord) { v.LaunchId = "launch:other" },
		"LlmSession echo":     func(v *NativeViewerReadinessRecord) { v.LlmSessionObjectKey = "session:other" },
		"Space echo":          func(v *NativeViewerReadinessRecord) { v.SpaceObjectKey = "space:other" },
		"manifest echo":       func(v *NativeViewerReadinessRecord) { v.ManifestObjectKey = "manifest:other" },
		"digest echo":         func(v *NativeViewerReadinessRecord) { v.ManifestDigest = "sha256:other" },
		"viewer echo":         func(v *NativeViewerReadinessRecord) { v.ViewerObjectKey = "viewer:other" },
		"profile echo":        func(v *NativeViewerReadinessRecord) { v.ViewerProfile = "other" },
		"scope echo":          func(v *NativeViewerReadinessRecord) { v.ResourceScopeLlmSessionObjectKey = "session:other" },
		"state echo":          func(v *NativeViewerReadinessRecord) { v.SelectedStateKey = "state:other" },
		"nonce echo":          func(v *NativeViewerReadinessRecord) { v.LaunchNonce = "nonce:other" },
		"resource tuple echo": func(v *NativeViewerReadinessRecord) { v.SpacewaveSessionRef.ProviderResourceId = "resource-2" },
		"provider tuple echo": func(v *NativeViewerReadinessRecord) { v.SpacewaveSessionRef.ProviderId = "provider-2" },
		"account tuple echo":  func(v *NativeViewerReadinessRecord) { v.SpacewaveSessionRef.ProviderAccountId = "account-2" },
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

// TestReadReadinessRecordLiveDoesNotWaitForEOF proves readiness becomes observable while the child retains the stream.
func TestReadReadinessRecordLiveDoesNotWaitForEOF(t *testing.T) {
	launch := testLaunch()
	readiness := testReadiness(launch)
	readiness.Status = NativeViewerReadinessStatus_NATIVE_VIEWER_READINESS_STATUS_READY
	readiness.ResourceRevision, readiness.ResourceCursor, readiness.FrameSequence = 1, 1, 1
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

// TestValidateSelectedState proves selected UI state rejects invalid identities, bounds, and cross-tab references.
func TestValidateSelectedState(t *testing.T) {
	base := &NativeViewerSelectedState{TabLlmSessionObjectKeys: []string{"glados/console", "logs"}, FocusedLlmSessionObjectKey: "glados/console", DraftsByLlmSessionObjectKey: map[string]string{"glados/console": "draft"}, ViewportsByLlmSessionObjectKey: map[string]uint32{"glados/console": 42}}
	if err := ValidateSelectedState(base); err != nil {
		t.Fatal(err)
	}
	cases := []func(*NativeViewerSelectedState){
		func(s *NativeViewerSelectedState) {
			s.TabLlmSessionObjectKeys = append(s.TabLlmSessionObjectKeys, "glados/console")
		},
		func(s *NativeViewerSelectedState) { s.FocusedLlmSessionObjectKey = "missing" },
		func(s *NativeViewerSelectedState) { s.DraftsByLlmSessionObjectKey["bad\x1bkey"] = "x" },
		func(s *NativeViewerSelectedState) {
			s.DraftsByLlmSessionObjectKey["glados/console"] = string(make([]byte, MaxDraftBytes+1))
		},
		func(s *NativeViewerSelectedState) {
			s.ViewportsByLlmSessionObjectKey["glados/console"] = MaxTranscriptOffset + 1
		},
		func(s *NativeViewerSelectedState) { s.ViewportsByLlmSessionObjectKey["missing"] = 1 },
		func(s *NativeViewerSelectedState) { s.SelectedView = 3 },
		func(s *NativeViewerSelectedState) { s.Theme = 3 },
	}
	for i, mutate := range cases {
		s := base.CloneVT()
		mutate(s)
		if ValidateSelectedState(s) == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}
