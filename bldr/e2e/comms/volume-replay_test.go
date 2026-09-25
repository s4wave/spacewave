//go:build !skip_e2e && !js

package comms

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// volumeBrowsers are the browsers the volume storage checks run in:
// VOLUME_BROWSERS, a comma-separated list, when set, otherwise Chromium and
// WebKit. Linux WebKit lacks OPFS sync access handles. The list may also
// name "safari" and "android"; see remote-browsers_test.go.
func volumeBrowsers(t *testing.T) []string {
	if list := os.Getenv("VOLUME_BROWSERS"); list != "" {
		return strings.Split(list, ",")
	}
	if runtime.GOOS == "linux" {
		t.Log("WebKit OPFS requires macOS; running Chromium only")
		return []string{"chromium"}
	}
	return []string{"chromium", "webkit"}
}

// TestGoScriptVolumeStorage runs the Device contract on the OPFS and
// IndexedDB devices and the record store contract on the IndexedDB store in
// a GoScript worker, each followed by a reopen that finds the same contents.
func TestGoScriptVolumeStorage(t *testing.T) {
	for _, browser := range volumeBrowsers(t) {
		t.Run(browser, func(t *testing.T) {
			ensureGoScriptFixtureWorker(t, &volumeReplayGoScriptFixtureWorker)
			results := runFixtureWith(t, browser, "goscript-volume-replay", fixtureRun{
				persistent: true,
				query:      "mode=check",
				timeout:    100 * time.Second,
			})
			if pass, ok := results["pass"].(bool); !ok || !pass {
				t.Fatalf("volume storage fixture failed: %v", results["detail"])
			}
			report := parseReport(t, results)
			for _, check := range []string{"opfs-device", "idb-device", "idb-records"} {
				if got := string(report.GetStringBytes(check)); got != "ok" {
					t.Errorf("%s: %s", check, got)
				}
			}
		})
	}
}

// volumeTargets are the engines the replay fixture opens by name.
var volumeTargets = []string{"e1-opfs", "e1-idb", "e5-idb", "e4-opfs", "e3-sqlite", "e1-opfs-t2"}

// TestGoScriptVolumeReplay replays the workload traces in WORKLOAD_TRACES, a
// directory of .trace files, against E1 on OPFS and IndexedDB, E5 on
// IndexedDB, format 3 on OPFS, SQLite in a device worker, and E1 on OPFS
// behind a device worker relay, from a GoScript worker, and logs each
// replay's report.
func TestGoScriptVolumeReplay(t *testing.T) {
	dir := os.Getenv("WORKLOAD_TRACES")
	if dir == "" {
		t.Skip("WORKLOAD_TRACES is not set")
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.trace"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no traces in %s", dir)
	}

	// Serve each trace as text records the worker parses.
	if err := os.MkdirAll(filepath.Join(distDir, "traces"), 0o755); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, path := range paths {
		recs, err := workload.ReadTraceRecords(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		name := strings.TrimSuffix(filepath.Base(path), ".trace")
		out := filepath.Join(distDir, "traces", name+".records")
		if err := os.WriteFile(out, workload.AppendRecords(nil, recs), 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}

	// Replay each trace against each target in its own page so each run
	// stays bounded.
	ensureGoScriptFixtureWorker(t, &volumeReplayGoScriptFixtureWorker)
	for _, browser := range volumeBrowsers(t) {
		for _, name := range names {
			for _, target := range volumeTargets {
				t.Run(browser+"/"+name+"/"+target, func(t *testing.T) {
					results := runFixtureWith(t, browser, "goscript-volume-replay", fixtureRun{
						persistent: true,
						query:      "mode=replay:" + name + "/" + target,
						timeout:    110 * time.Second,
					})
					if pass, ok := results["pass"].(bool); !ok || !pass {
						t.Fatalf("volume replay fixture failed: %v", results["detail"])
					}
					for _, rep := range parseReport(t, results).GetArray() {
						t.Logf("REPORT %s %s", browser, rep)
						if msg := rep.GetStringBytes("error"); len(msg) != 0 {
							t.Errorf("%s %s %s: %s", name, target, rep.GetStringBytes("policy"), msg)
						}
					}
				})
			}
		}
	}
}

// parseReport parses the JSON report the volume fixture publishes.
func parseReport(t *testing.T, results map[string]any) *fastjson.Value {
	raw, _ := results["report"].(string)
	report, err := fastjson.Parse(raw)
	if err != nil {
		t.Fatalf("volume fixture report: %v", err)
	}
	return report
}
