package device_policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRawPolicyFile writes raw policy JSON at the state root the way an
// operator editor would.
func writeRawPolicyFile(t *testing.T, stateRoot, data string) {
	t.Helper()
	dir := filepath.Join(stateRoot, StateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, StateFile), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestReadFileRejectsInvalidForgeWorkerEnvelope proves the declared-capacity
// fail-before: a forge-worker declaration missing a required member is
// rejected at read time with the forge-worker validation error and never
// decodes as an available envelope.
func TestReadFileRejectsInvalidForgeWorkerEnvelope(t *testing.T) {
	stateRoot := t.TempDir()
	writeRawPolicyFile(t, stateRoot,
		`{"forgeWorker":{"workerObjectKey":"worker/a","milliCpu":0,`+
			`"memoryBytes":4294967296,"backends":["docker"]}}`)
	_, err := ReadFile(stateRoot)
	if err == nil {
		t.Fatal("invalid forge-worker envelope must be rejected at read time")
	}
	if !strings.Contains(err.Error(), "device policy forge-worker") {
		t.Fatalf("expected forge-worker validation error, got %v", err)
	}
}

// TestReadFileAcceptsCompleteForgeWorkerEnvelope pins that a complete
// declaration reads cleanly, so the rejection above is about validity and not
// the presence of the section.
func TestReadFileAcceptsCompleteForgeWorkerEnvelope(t *testing.T) {
	stateRoot := t.TempDir()
	writeRawPolicyFile(t, stateRoot,
		`{"forgeWorker":{"workerObjectKey":"worker/a","milliCpu":1000,`+
			`"memoryBytes":1073741824,"backends":["docker","v86"]}}`)
	if _, err := ReadFile(stateRoot); err != nil {
		t.Fatalf("complete forge-worker envelope must read cleanly: %v", err)
	}
}

// validForgeWorkerJSON is a complete declared envelope in the wire form the
// generated codecs accept.
const validForgeWorkerJSON = `{"forgeWorker":{"workerObjectKey":"worker/a",` +
	`"milliCpu":1000,"memoryBytes":1073741824,"backends":["docker","v86"]}}`

// TestValidateRejectsPartialForgeWorkerEnvelopes proves all-or-nothing: every
// partial declaration fails Validate with the forge-worker error, and only a
// complete envelope passes.
func TestValidateRejectsPartialForgeWorkerEnvelopes(t *testing.T) {
	base := func() *ForgeWorkerPolicy {
		return &ForgeWorkerPolicy{
			WorkerObjectKey: "worker/a",
			MilliCpu:        1000,
			MemoryBytes:     1 << 30,
			Backends:        []string{"docker"},
		}
	}
	cases := map[string]*ForgeWorkerPolicy{
		"missing key":        {MilliCpu: 1000, MemoryBytes: 1 << 30, Backends: []string{"docker"}},
		"zero milli_cpu":     {WorkerObjectKey: "worker/a", MemoryBytes: 1 << 30, Backends: []string{"docker"}},
		"zero memory_bytes":  {WorkerObjectKey: "worker/a", MilliCpu: 1000, Backends: []string{"docker"}},
		"empty backends":     {WorkerObjectKey: "worker/a", MilliCpu: 1000, MemoryBytes: 1 << 30},
		"blank backend":      {WorkerObjectKey: "worker/a", MilliCpu: 1000, MemoryBytes: 1 << 30, Backends: []string{" "}},
		"spaced backend":     {WorkerObjectKey: "worker/a", MilliCpu: 1000, MemoryBytes: 1 << 30, Backends: []string{"docker host"}},
		"comma backend":      {WorkerObjectKey: "worker/a", MilliCpu: 1000, MemoryBytes: 1 << 30, Backends: []string{"docker,host"}},
		"duplicate backends": {WorkerObjectKey: "worker/a", MilliCpu: 1000, MemoryBytes: 1 << 30, Backends: []string{"docker", "docker"}},
	}
	for name, fw := range cases {
		err := Validate(&DevicePolicy{Revision: 1, ForgeWorker: fw})
		if err == nil || !strings.Contains(err.Error(), "device policy forge-worker") {
			t.Fatalf("%s: expected forge-worker validation error, got %v", name, err)
		}
	}

	if err := Validate(&DevicePolicy{Revision: 1, ForgeWorker: base()}); err != nil {
		t.Fatalf("complete envelope must validate: %v", err)
	}
}

// TestValidateAcceptsAbsentSection pins the other half of all-or-nothing: no
// forge_worker section at all decodes and validates as plain local policy.
func TestValidateAcceptsAbsentSection(t *testing.T) {
	if err := Validate(&DevicePolicy{Revision: 1}); err != nil {
		t.Fatalf("absent section must validate: %v", err)
	}
}

// TestForgeWorkerEnvelopeJSONRoundTrip pins that the declared envelope
// survives the file codec unchanged and that an unknown-section document
// decodes to a zero section (legacy tolerance).
func TestForgeWorkerEnvelopeJSONRoundTrip(t *testing.T) {
	var policy DevicePolicy
	if err := policy.UnmarshalJSON([]byte(validForgeWorkerJSON)); err != nil {
		t.Fatal(err)
	}
	fw := policy.GetForgeWorker()
	if fw == nil || fw.GetWorkerObjectKey() != "worker/a" || fw.GetMilliCpu() != 1000 ||
		fw.GetMemoryBytes() != 1<<30 || len(fw.GetBackends()) != 2 {
		t.Fatalf("roundtrip lost declared fields: %+v", &policy)
	}
	if err := Validate(&policy); err != nil {
		t.Fatalf("decoded envelope must validate: %v", err)
	}

	data, err := policy.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var reparsed DevicePolicy
	if err := reparsed.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}
	if !reparsed.EqualVT(&policy) {
		t.Fatalf("marshal lost fields: %s", string(data))
	}

	var legacy DevicePolicy
	if err := legacy.UnmarshalJSON([]byte(`{"revision":2}`)); err != nil {
		t.Fatal(err)
	}
	if legacy.GetForgeWorker() != nil {
		t.Fatalf("legacy document must decode without a section: %+v", &legacy)
	}
	if err := Validate(&legacy); err != nil {
		t.Fatalf("legacy policy must validate: %v", err)
	}
}
