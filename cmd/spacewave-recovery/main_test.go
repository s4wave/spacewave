//go:build !js && !wasip1

package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	space_recovery "github.com/s4wave/spacewave/core/space/recovery"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/coord"
	coord_filelock "github.com/s4wave/spacewave/db/coord/filelock"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	recovery "github.com/s4wave/spacewave/db/world/recovery"
)

// countOptions selects exactly the native constructor registry used by the CLI.
func countOptions() recovery.Options {
	return recovery.Options{Lookup: optypes.LookupBlockType, Witness: space_recovery.ValidateWitness, InterpretationID: "spacewave-native-registry-v1"}
}

// sourceStamp records metadata whose preservation the immutable reader promises.
func sourceStamp(t *testing.T, path string) []any {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	var change any
	for _, field := range []string{"Ctim", "Ctimespec"} {
		if value := stat.FieldByName(field); value.IsValid() {
			change = value.Interface()
			break
		}
	}
	return []any{info.Size(), info.Mode(), info.ModTime(), stat.FieldByName("Dev").Interface(), stat.FieldByName("Ino").Interface(), change}
}

// assertPreserved compares bytes and all promised metadata, including native root/list bytes.
func assertPreserved(t *testing.T, path string, before []byte, stamp []any) {
	t.Helper()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !reflect.DeepEqual(stamp, sourceStamp(t, path)) {
		t.Fatal("scan changed source bytes, inode, size, mode, mtime or ctime")
	}
}

// TestRecoveryCount runs the real CLI on native persisted evidence and checks its units.
func TestRecoveryCount(t *testing.T) {
	// Capture source bytes and metadata after fixture construction has closed every writer.
	fixture := makeFixture(t)
	source := fixture.source
	before, err := os.ReadFile(source.GetPath())
	if err != nil {
		t.Fatal(err)
	}
	stamp := sourceStamp(t, source.GetPath())
	scratch := filepath.Join(t.TempDir(), "count.sqlite")
	app := buildApp()
	var output bytes.Buffer
	app.Writer = &output
	if err := app.Run([]string{"spacewave-recovery", "--help"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "--source capture.pb --scratch work.sqlite") {
		t.Fatal("help omitted the count example")
	}
	output.Reset()
	if err := app.Run([]string{"spacewave-recovery", "--source", writeManifest(t, source), "--scratch", scratch}); err != nil {
		t.Fatal(err)
	}
	result, err := recovery.ReadResult(scratch)
	if err != nil {
		t.Fatal(err)
	}
	count := result.GetCounts()
	want := fixture.expected
	if !count.EqualVT(want) {
		t.Fatalf("fixture count mismatch\ngot: %v\nwant: %v", count, want)
	}
	if result.GetPhase() != recovery.Phase_PHASE_COUNTED || len(result.GetGaps()) == 0 || !strings.Contains(output.String(), "Coverage:") || !strings.Contains(output.String(), "Confirmed refusal losses") {
		t.Fatal("CLI omitted completed state or coverage")
	}
	if count.GetOrphanBlocks() == 0 && count.GetUnknownReachability() == 0 {
		t.Fatal("fixture orphan inventory was omitted")
	}
	assertPreserved(t, source.GetPath(), before, stamp)
	t.Log(output.String())

	// The saved report reads without touching or requiring the source file.
	if err := os.Rename(source.GetPath(), source.GetPath()+".offline"); err != nil {
		t.Fatal(err)
	}
	report := buildApp()
	report.Writer = io.Discard
	if err := report.Run([]string{"spacewave-recovery", "--scratch", scratch, "--report"}); err != nil {
		t.Fatal(err)
	}
}

// TestRecoveryResume kills after durable commits and injects resumable bounded-step failures.
func TestRecoveryResume(t *testing.T) {
	// A child pauses exactly after a committed step so SIGKILL tests disk durability.
	if manifest := os.Getenv("B8_RECOVERY_CHILD"); manifest != "" {
		data, err := os.ReadFile(manifest)
		if err != nil {
			t.Fatal(err)
		}
		source := &recovery.Source{}
		if err := source.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
		phase, _ := strconv.Atoi(os.Getenv("B8_RECOVERY_PHASE"))
		opts := countOptions()
		boundary := func(result *recovery.Result) error {
			if (phase == 0 && result.GetCounts().GetPhysicalLocations() >= 5) || (phase != 0 && int(result.GetPhase()) == phase) {
				_, _ = os.Stdout.WriteString("B8 BOUNDARY\n")
				select {}
			}
			return nil
		}
		if phase == 0 {
			opts.BeforeCommit = boundary
		} else {
			opts.Committed = boundary
		}
		if err := runCount(t.Context(), io.Discard, source, os.Getenv("B8_RECOVERY_SCRATCH"), opts); err != nil {
			t.Fatal(err)
		}
		return
	}

	// Fresh and resumed reports must have identical aggregate counts and coverage.
	fixture := makeFixture(t)
	before, err := os.ReadFile(fixture.source.GetPath())
	if err != nil {
		t.Fatal(err)
	}
	stamp := sourceStamp(t, fixture.source.GetPath())
	freshPath := filepath.Join(t.TempDir(), "fresh.sqlite")
	if err := runCount(t.Context(), io.Discard, fixture.source, freshPath, countOptions()); err != nil {
		t.Fatal(err)
	}
	fresh, err := recovery.ReadResult(freshPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []recovery.Phase{recovery.Phase_PHASE_UNKNOWN, recovery.Phase_PHASE_INVENTORY, recovery.Phase_PHASE_INTERPRET, recovery.Phase_PHASE_TRAVERSE, recovery.Phase_PHASE_COUNTED} {
		t.Run("kill-"+strconv.Itoa(int(phase)), func(t *testing.T) {
			scratch := filepath.Join(t.TempDir(), "killed.sqlite")
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestRecoveryResume$", "-test.timeout=60s")
			cmd.Env = append(os.Environ(), "B8_RECOVERY_CHILD="+writeManifest(t, fixture.source), "B8_RECOVERY_SCRATCH="+scratch, "B8_RECOVERY_PHASE="+strconv.Itoa(int(phase)))
			pipe, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			scanner := bufio.NewScanner(pipe)
			ready := false
			for scanner.Scan() {
				if scanner.Text() == "B8 BOUNDARY" {
					ready = true
					break
				}
			}
			if !ready {
				_ = cmd.Wait()
				t.Fatal("child did not reach durable boundary")
			}
			if second, err := recovery.NewScanner(t.Context(), fixture.source, scratch, countOptions()); err == nil {
				second.Close()
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatal("another process acquired an owned scratch run")
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = cmd.Wait()
			if phase == recovery.Phase_PHASE_UNKNOWN {
				// The kill landed inside a batch: none of its prepared steps
				// may be durable.
				killed, err := recovery.ReadResult(scratch)
				if err != nil {
					t.Fatal(err)
				}
				if killed.GetCounts().GetPhysicalLocations() >= 5 {
					t.Fatalf("killed batch left %d durable locations", killed.GetCounts().GetPhysicalLocations())
				}
			}
			if err := runCount(t.Context(), io.Discard, fixture.source, scratch, countOptions()); err != nil {
				t.Fatal(err)
			}
			result, err := recovery.ReadResult(scratch)
			if err != nil {
				t.Fatal(err)
			}
			if !result.EqualVT(fresh) {
				t.Fatal("kill/resume differs from fresh count")
			}
			assertPreserved(t, fixture.source.GetPath(), before, stamp)
		})
	}
	// A final observer failure leaves the already durable terminal report counted.
	terminalPath := filepath.Join(t.TempDir(), "terminal.sqlite")
	terminalFailure := errors.New("synthetic terminal observer")
	terminalOptions := countOptions()
	terminalOptions.Committed = func(result *recovery.Result) error {
		if result.GetPhase() == recovery.Phase_PHASE_COUNTED {
			return terminalFailure
		}
		return nil
	}
	if err := runCount(t.Context(), io.Discard, fixture.source, terminalPath, terminalOptions); !errors.Is(err, terminalFailure) {
		t.Fatalf("terminal observer: %v", err)
	}
	terminal, err := recovery.ReadResult(terminalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !terminal.EqualVT(fresh) {
		t.Fatal("observer demoted or altered a durable completed report")
	}
	assertPreserved(t, fixture.source.GetPath(), before, stamp)

	for _, fault := range []string{"read", "decode", "scratch-full"} {
		t.Run(fault, func(t *testing.T) {
			scratch := filepath.Join(t.TempDir(), "fault.sqlite")
			injected := errors.New("synthetic " + fault)
			opts := countOptions()
			var committed uint64
			opts.Committed = func(result *recovery.Result) error { committed = result.GetCounts().GetPhysicalLocations(); return nil }
			fired := false
			switch fault {
			case "read":
				opts.BeforeRead = func() error {
					if committed >= 5 {
						return injected
					}
					return nil
				}
			case "decode":
				opts.BeforeDecode = func() error { return injected }
			case "scratch-full":
				opts.BeforeCommit = func(result *recovery.Result) error {
					if !fired && result.GetCounts().GetPhysicalLocations() >= 5 {
						fired = true
						return injected
					}
					return nil
				}
			}
			scanner, err := recovery.NewScanner(t.Context(), fixture.source, scratch, opts)
			if err != nil {
				t.Fatal(err)
			}
			result, err := scanner.Run(t.Context())
			if !errors.Is(err, injected) || result.GetPhase() != recovery.Phase_PHASE_PAUSED {
				t.Fatalf("fault published nonprovisional result: %v %v", result, err)
			}
			// A scratch failure inside a batch rolls the whole batch back; the
			// other faults strike after committed inventory.
			if locations := result.GetCounts().GetPhysicalLocations(); (fault == "scratch-full") != (locations < 5) || locations != committed {
				t.Fatalf("%s fault kept %d locations, last commit had %d", fault, locations, committed)
			}
			if err := scanner.Close(); err != nil {
				t.Fatal(err)
			}
			if err := runCount(t.Context(), io.Discard, fixture.source, scratch, countOptions()); err != nil {
				t.Fatal(err)
			}
			result, err = recovery.ReadResult(scratch)
			if err != nil {
				t.Fatal(err)
			}
			if !result.EqualVT(fresh) {
				t.Fatal("fault/resume differs from fresh count")
			}
			assertPreserved(t, fixture.source.GetPath(), before, stamp)
		})
	}
}

// TestRecoveryEvidence rejects unrelated acceptance, fake height-only cover and changed input.
func TestRecoveryEvidence(t *testing.T) {
	// Native evidence changes are made only during synthetic fixture setup.
	for _, scenario := range []string{"preexisting", "different-base", "wrong-author-hash", "no-current", "identity", "lookup", "duplicate-table", "owned", "duplicate-world", "live-hole", "untyped-root", "root-bucket"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := makeFixture(t)
			source := fixture.source
			scratch := filepath.Join(t.TempDir(), "evidence.sqlite")
			if scenario == "preexisting" || scenario == "wrong-author-hash" || scenario == "different-base" {
				db, err := sql.Open("sqlite3", source.GetPath())
				if err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "preexisting":
					data := []byte(nil)
					if err := db.QueryRow("SELECT value FROM volume WHERE key=?", []byte("native/accepted")).Scan(&data); err != nil {
						t.Fatal(err)
					}
					cursor := &space_world.ReplayCursor{}
					if err := cursor.UnmarshalVT(data); err != nil {
						t.Fatal(err)
					}
					cursor.Base.HeadRef = source.GetWorlds()[1].GetRef()
					data, err = cursor.MarshalVT()
					if err != nil {
						t.Fatal(err)
					}
					if _, err := db.Exec("UPDATE volume SET value=? WHERE key=?", data, []byte("native/accepted")); err != nil {
						t.Fatal(err)
					}
				case "different-base":
					var data []byte
					if err := db.QueryRow("SELECT value FROM volume WHERE key=?", []byte("native/refused")).Scan(&data); err != nil {
						t.Fatal(err)
					}
					cursor := &space_world.ReplayCursor{}
					if err := cursor.UnmarshalVT(data); err != nil {
						t.Fatal(err)
					}
					cursor.Base.HeadRef = source.GetWorlds()[3].GetRef()
					cursor.Head.HeadRef = source.GetWorlds()[3].GetRef()
					cursor.Outcomes[0].World = source.GetWorlds()[3].GetRef().GetRootRef()
					data, err = cursor.MarshalVT()
					if err != nil {
						t.Fatal(err)
					}
					if _, err := db.Exec("UPDATE volume SET value=? WHERE key=?", data, []byte("native/refused")); err != nil {
						t.Fatal(err)
					}
				default:
					data := []byte(nil)
					if err := db.QueryRow("SELECT value FROM volume WHERE key=?", []byte("native/checkpoint")).Scan(&data); err != nil {
						t.Fatal(err)
					}
					checkpoint := &sobject.SOCheckpointInner{}
					if err := checkpoint.UnmarshalVT(data); err != nil {
						t.Fatal(err)
					}
					checkpoint.Height = 1000000
					checkpoint.Authors[0].OpHash = bytes.Repeat([]byte{9}, 32)
					data, err = checkpoint.MarshalVT()
					if err != nil {
						t.Fatal(err)
					}
					if _, err := db.Exec("UPDATE volume SET value=? WHERE key=?", data, []byte("native/checkpoint")); err != nil {
						t.Fatal(err)
					}
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "duplicate-world" {
				source.Worlds = append(source.Worlds, source.Worlds[2].CloneVT())
			}
			if scenario == "untyped-root" || scenario == "root-bucket" {
				db, err := sql.Open("sqlite3", source.GetPath())
				if err != nil {
					t.Fatal(err)
				}
				ref := fixture.liveChild
				if scenario == "root-bucket" {
					ref = source.GetWorlds()[3].GetRef().GetRootRef()
				}
				data, err := (&block_gc.RootEntry{Ref: block_gc.BlockKey(ref)}).MarshalVT()
				if err != nil {
					t.Fatal(err)
				}
				for _, table := range source.GetTables() {
					if _, err := db.Exec("INSERT INTO "+table+" VALUES (?,?)", []byte{2, 'R', 1, 'x', 'y'}, data); err != nil {
						t.Fatal(err)
					}
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}

			if scenario == "live-hole" {
				db, err := sql.Open("sqlite3", source.GetPath())
				if err != nil {
					t.Fatal(err)
				}
				key, err := fixture.liveChild.MarshalKey()
				if err != nil {
					t.Fatal(err)
				}
				key = store_kvkey.NewDefaultKVKey().GetBlockKey(key)
				for _, table := range source.GetTables() {
					if _, err := db.Exec("DELETE FROM "+table+" WHERE key=?", key); err != nil {
						t.Fatal(err)
					}
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "no-current" {
				for _, input := range source.Worlds {
					input.Current = false
				}
			}
			if scenario == "owned" {
				first, err := recovery.NewScanner(t.Context(), source, scratch, countOptions())
				if err != nil {
					t.Fatal(err)
				}
				defer first.Close()
				second, err := recovery.NewScanner(t.Context(), source, scratch, countOptions())
				if err == nil {
					second.Close()
					t.Fatal("second active count acquired the scratch run")
				}
				return
			}
			if scenario == "duplicate-table" {
				source.Tables = append(source.Tables, source.Tables[0])
				if scanner, err := recovery.NewScanner(t.Context(), source, scratch, countOptions()); err == nil {
					scanner.Close()
					t.Fatal("duplicate location accepted")
				}
				return
			}
			if scenario == "identity" || scenario == "lookup" {
				opts := countOptions()
				opts.BeforeStep = func(recovery.Phase) error { return context.Canceled }
				scanner, err := recovery.NewScanner(t.Context(), source, scratch, opts)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = scanner.Run(t.Context())
				scanner.Close()
				if scenario == "identity" {
					info, err := os.Stat(source.GetPath())
					if err != nil {
						t.Fatal(err)
					}
					data, err := os.ReadFile(source.GetPath())
					if err != nil {
						t.Fatal(err)
					}
					replacement := source.GetPath() + ".replacement"
					if err := os.WriteFile(replacement, data, info.Mode()); err != nil {
						t.Fatal(err)
					}
					if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(replacement, source.GetPath()); err != nil {
						t.Fatal(err)
					}
				}
				nextOpts := countOptions()
				if scenario == "lookup" {
					nextOpts.InterpretationID = "changed-registry"
				}
				scanner, err = recovery.NewScanner(t.Context(), source, scratch, nextOpts)
				if err != nil {
					t.Fatal(err)
				}
				result, err := scanner.Run(t.Context())
				scanner.Close()
				if err == nil || result.GetPhase() != recovery.Phase_PHASE_STALE {
					t.Fatal("changed capture or interpretation resumed old work")
				}
				return
			}
			if err := runCount(t.Context(), io.Discard, source, scratch, countOptions()); err != nil {
				t.Fatal(err)
			}
			result, err := recovery.ReadResult(scratch)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "duplicate-world" {
				if !result.GetCounts().EqualVT(fixture.expected) {
					t.Fatal("shared historical World inflated counts")
				}
				return
			}
			if scenario == "untyped-root" || scenario == "root-bucket" {
				if result.GetCounts().GetOrphanBlocks() != 0 || result.GetCounts().GetUnknownReachability() == 0 {
					t.Fatal("hash-only root certified complete typed reachability")
				}
				return
			}

			if scenario == "live-hole" {
				if result.GetCounts().GetLiveHoles() != 1 || result.GetCounts().GetOrphanBlocks() != 0 || result.GetCounts().GetUnknownReachability() == 0 {
					t.Fatal("missing live descendant certified orphan reachability")
				}
				return
			}
			if result.GetCounts().GetConfirmedRefusalLosses() != 0 {
				t.Fatal("unproven causal history confirmed")
			}
			if scenario == "no-current" && result.GetCounts().GetAttributableVersions() != 0 {
				t.Fatal("missing current World certified version absence")
			}
		})
	}
}

// TestRecoverySourceAliases refuses writable SQLite and lease targets that alias the capture.
func TestRecoverySourceAliases(t *testing.T) {
	// Each adversary starts from a fresh synthetic capture and records metadata after setup.
	for _, kind := range []string{"wal-name", "sidecar-hardlink", "sidecar-symlink", "lease-hardlink", "scratch-symlink", "dsn"} {
		t.Run(kind, func(t *testing.T) {
			fixture := makeFixture(t)
			source := fixture.source
			scratch := filepath.Join(t.TempDir(), "scratch.sqlite")
			switch kind {
			case "wal-name":
				next := scratch + "-wal"
				if err := os.Rename(source.GetPath(), next); err != nil {
					t.Fatal(err)
				}
				source.Path = next
			case "sidecar-hardlink":
				if err := os.Link(source.GetPath(), scratch+"-wal"); err != nil {
					t.Fatal(err)
				}
			case "sidecar-symlink":
				if err := os.Symlink(source.GetPath(), scratch+"-journal"); err != nil {
					t.Fatal(err)
				}
			case "lease-hardlink":
				coordinator := coord_filelock.NewCoordinator(filepath.Dir(scratch), scratch, nil)
				lease := coordinator.LockPath(coord.Scope{Key: "recovery-count"})
				if err := os.MkdirAll(filepath.Dir(lease), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(source.GetPath(), lease); err != nil {
					t.Fatal(err)
				}
			case "scratch-symlink":
				if err := os.Symlink(source.GetPath(), scratch); err != nil {
					t.Fatal(err)
				}
			case "dsn":
				scratch = source.GetPath() + "?ignored=1"
			}
			before, err := os.ReadFile(source.GetPath())
			if err != nil {
				t.Fatal(err)
			}
			stamp := sourceStamp(t, source.GetPath())
			scanner, err := recovery.NewScanner(t.Context(), source, scratch, countOptions())
			if err == nil {
				scanner.Close()
				t.Fatal("writable scratch alias was accepted")
			}
			assertPreserved(t, source.GetPath(), before, stamp)
		})
	}
}

// TestRecoveryInterpretationOrder keeps conflicting historical constructors partial in either order.
func TestRecoveryInterpretationOrder(t *testing.T) {
	// Both graphs are native, and changing a graph type leaves the Object metadata unchanged.
	fixture := makeFixtureWithConflict(t, true)
	before, err := os.ReadFile(fixture.source.GetPath())
	if err != nil {
		t.Fatal(err)
	}
	stamp := sourceStamp(t, fixture.source.GetPath())
	var counts *recovery.Counts
	for _, reverse := range []bool{false, true} {
		source := fixture.source.CloneVT()
		if reverse {
			source.Worlds[1], source.Worlds[2] = source.Worlds[2], source.Worlds[1]
		}
		scratch := filepath.Join(t.TempDir(), "order.sqlite")
		if err := runCount(t.Context(), io.Discard, source, scratch, countOptions()); err != nil {
			t.Fatal(err)
		}
		result, err := recovery.ReadResult(scratch)
		if err != nil {
			t.Fatal(err)
		}
		if result.GetCounts().GetPartialVersions() != 3 {
			t.Fatalf("conflicting exact version was certified complete: %v", result.GetCounts())
		}
		if counts != nil && !counts.EqualVT(result.GetCounts()) {
			t.Fatal("World declaration order changed classification")
		}
		counts = result.GetCounts()
		assertPreserved(t, source.GetPath(), before, stamp)
	}
}
