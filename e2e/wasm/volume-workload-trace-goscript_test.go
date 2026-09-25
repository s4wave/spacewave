//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// volumeWorkloadGCWait covers one full sweep of the browser volume's default
// one-minute GC interval (db/volume/controller/config.go defaultGCInterval).
const volumeWorkloadGCWait = 65 * time.Second

// TestGoScriptVolumeWorkloadTraces captures the spacewave-core runtime traces
// of a Drive import, a large-file read, and the GC sweep that follows a
// delete. Each trace carries the volume workload records the browser storage
// engine replays; see db/volume/workload.
func TestGoScriptVolumeWorkloadTraces(t *testing.T) {
	skipTraceServiceWhenDisabled(t)

	sess := harness(t).NewCleanSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()

	scenario := CreateDriveScenario(t, harness(t), sess)
	page := scenario.GetSession().Page()
	WaitForDriveReady(t, harness(t), page)

	ctx, cancel := context.WithTimeout(t.Context(), 110*time.Second)
	defer cancel()

	// capture brackets one interaction and writes its trace beside the
	// test's other artifacts.
	capture := func(name string, fn func()) {
		data, err := sess.CaptureTrace(ctx, "volume-workload-"+name, func(context.Context) error {
			fn()
			return nil
		})
		if err != nil {
			t.Fatalf("capture %s trace: %v", name, err)
		}
		path := filepath.Join(filepath.Dir(TraceArtifactPath(t)), "volume-workload-"+name+".trace")
		if err := WriteTraceArtifact(path, data); err != nil {
			t.Fatalf("write %s trace: %v", name, err)
		}
		t.Logf("%s trace written to %s (%d bytes)", name, path, len(data))
	}

	// Import the upload fixture, which ends with the 8 MiB file.
	files := driveUploadFixtureFiles()
	large := files[len(files)-1]
	capture("import", func() {
		UploadViaPicker(t, page, files)
		for _, file := range files {
			waitForDriveEntry(t, page, file.Name)
		}
	})

	// Read the large file back through the plugin's HTTP path.
	capture("read", func() {
		verifyUploadedFile(t, scenario, page, large)
	})

	// Delete the large file and hold the trace across one GC sweep.
	capture("gc", func() {
		deleteDriveEntryViaContextMenu(t, page, large.Name)
		select {
		case <-ctx.Done():
		case <-time.After(volumeWorkloadGCWait):
		}
	})

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report during workload traces: %+v", report)
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop during workload traces: %+v", report)
	}
}
