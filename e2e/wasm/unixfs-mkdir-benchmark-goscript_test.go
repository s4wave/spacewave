//go:build !skip_e2e && !js

package wasm

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/gitroot"
)

func recordUnixFSMkdirTiming(t testing.TB, result map[string]any) {
	t.Helper()

	rawSamples, ok := result["mkdirSamplesMs"].([]any)
	if !ok || len(rawSamples) != 3 {
		t.Fatalf("UnixFS mkdir timing samples = %#v, want three samples", result["mkdirSamplesMs"])
	}
	samples := make([]float64, len(rawSamples))
	for idx, raw := range rawSamples {
		sample, ok := raw.(float64)
		if !ok || sample <= 0 {
			t.Fatalf("UnixFS mkdir timing sample %d = %#v, want a positive duration", idx, raw)
		}
		samples[idx] = sample
	}

	var arena fastjson.Arena
	measurement := arena.NewObject()
	measurement.Set("compiler", arena.NewString(string(E2EWasmCompilerGoScript)))
	measurement.Set("operation", arena.NewString("unixfs-mkdir-all-in-space"))
	measurementSamples := arena.NewArray()
	for idx, sample := range samples {
		measurementSamples.SetArrayItem(
			idx,
			arena.NewNumberString(strconv.FormatFloat(sample, 'f', 6, 64)),
		)
	}
	measurement.Set("samplesMs", measurementSamples)

	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		t.Fatalf("find repo root for UnixFS mkdir measurements: %v", err)
	}
	artifactPath := filepath.Join(
		repoRoot,
		".bldr",
		"e2e-goscript-unixfs-mkdir",
		"benchmark.json",
	)
	if err := WriteTraceArtifact(artifactPath, append(measurement.MarshalTo(nil), '\n')); err != nil {
		t.Fatalf("write UnixFS mkdir measurements: %v", err)
	}
	t.Logf(
		"goscript UnixFS mkdir samples: %.3fms %.3fms %.3fms",
		samples[0],
		samples[1],
		samples[2],
	)
}
