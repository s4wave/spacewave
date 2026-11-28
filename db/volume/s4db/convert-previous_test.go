//go:build !windows && !js && !wasip1

package volume_s4db

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/s4db"
	"github.com/s4wave/spacewave/db/s4db/filelock"
	"github.com/s4wave/spacewave/db/s4db/s4dbv1"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

// previousFixtures is the path to the files the previous engine wrote.
const previousFixtures = "../../s4db/s4dbv1/testdata"

// previousHolderEnv selects the previous-format file a test process holds
// open until its standard input closes.
const previousHolderEnv = "SPACEWAVE_S4DBV1_HOLDER"

// previousOpenerEnv selects the previous-format file a test process opens
// as the previous engine does, waiting for a line on standard input between
// opening the file and taking its locks.
const previousOpenerEnv = "SPACEWAVE_S4DBV1_OPENER"

// previousRow is one expected pair: its key, length and value hash.
type previousRow struct {
	key  string
	n    int
	hash string
}

// previousVolume copies the previous-format fixture name into a fresh
// directory as the Volume file and returns its path.
func previousVolume(t *testing.T, name string) string {
	// Copy the fixture.
	b, err := os.ReadFile(filepath.Join(previousFixtures, name))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// expectedRows reads the fixture's expected pairs.
func expectedRows(t *testing.T) []previousRow {
	// Parse each tab-separated line.
	f, err := os.Open(filepath.Join(previousFixtures, "mixed.expected.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows []previousRow
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), "\t")
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, previousRow{key: fields[0], n: n, hash: fields[2]})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return rows
}

// checkConverted checks that path opens in the current format and holds
// exactly want, that the previous file is kept as the pre-rewrite link in the
// previous format, and that no conversion file is left beside them.
func checkConverted(t *testing.T, path string, want []previousRow) {
	// Read every pair of the converted file.
	ctx := context.Background()
	db, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Iterate a read transaction over all of it.
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	it := tx.Iterate(ctx, nil, true, false)
	defer it.Close()

	// Compare each pair with the expected row.
	n := 0
	for it.Next() {
		v, err := it.Value()
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(v)
		got := previousRow{key: string(it.Key()), n: len(v), hash: hex.EncodeToString(sum[:])}
		if n >= len(want) || got != want[n] {
			t.Fatalf("pair %d: got %+v", n, got)
		}
		n++
	}
	if err := it.Err(); err != nil || n != len(want) {
		t.Fatalf("read %d pairs, want %d: %v", n, len(want), err)
	}

	// The previous file is kept and nothing else is left.
	r, err := s4dbv1.Open(path + preRewriteSuffix)
	if err != nil {
		t.Fatalf("pre-rewrite: %v", err)
	}
	r.Close()
	for _, suffix := range []string{convertingSuffix, convertedMarkerSuffix} {
		if _, err := os.Stat(path + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s left: %v", suffix, err)
		}
	}
}

// convertClosed converts path and closes the fence, which a test needs
// before it opens the backup in this process.
func convertClosed(ctx context.Context, le *logrus.Entry, path string) error {
	fence, err := convertPrevious(ctx, le, path)
	if fence != nil {
		err = errors.Join(err, fence.Close())
	}
	return err
}

// sourceID returns the identity a claim records for src.
func sourceID(t *testing.T, src *s4dbv1.Reader) string {
	fi, err := src.Stat()
	if err != nil {
		t.Fatal(err)
	}
	id, err := fileIdentity(fi)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// verifiedButNotInstalled runs the conversion through the marker and stops
// before the install, as a crash there would, returning the new file's
// details.
func verifiedButNotInstalled(t *testing.T, path string) os.FileInfo {
	// Build and verify.
	ctx := context.Background()
	src, err := s4dbv1.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := buildConverted(ctx, src, sourceID(t, src), path+convertingSuffix, path+convertedMarkerSuffix, nil); err != nil {
		t.Fatal(err)
	}

	// Describe the new file.
	fi, err := os.Stat(path + convertingSuffix)
	if err != nil {
		t.Fatal(err)
	}
	return fi
}

// TestConvertPrevious converts the previous engine's fixture at open and
// from each state a crash can leave: a stale unverified file, a verified
// file with its marker, the link made but not the rename, and a torn file
// under a marker. It also checks the refusals: a file another process holds
// and a disk without room.
func TestConvertPrevious(t *testing.T) {
	// Hold the file open through the previous-format reader when started as
	// the holder, since record locks on some systems belong to the process.
	if path := os.Getenv(previousHolderEnv); path != "" {
		holdPrevious(t, path)
		return
	}
	if path := os.Getenv(previousOpenerEnv); path != "" {
		openAsPrevious(t, path)
		return
	}
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	want := expectedRows(t)

	// A fresh conversion, and the file opens without converting again.
	t.Run("fresh", func(t *testing.T) {
		// Convert twice.
		path := previousVolume(t, "mixed.s4db")
		if err := convertClosed(ctx, le, path); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, want)
		if err := convertClosed(ctx, le, path); err != nil {
			t.Fatal(err)
		}
	})

	// A file of a conversion's names that no claim accounts for is refused
	// and kept, and so is a marker that is not a claim of this source.
	t.Run("unowned files", func(t *testing.T) {
		for _, name := range []string{convertingSuffix, convertedMarkerSuffix} {
			// Leave a file the conversion did not write, then convert.
			path := previousVolume(t, "mixed.s4db")
			if err := os.WriteFile(path+name, []byte("someone else's"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := convertClosed(ctx, le, path); err == nil {
				t.Fatalf("%s: converted beside an unowned file", name)
			}

			// The file and the source are as they were.
			if b, err := os.ReadFile(path + name); err != nil || string(b) != "someone else's" {
				t.Fatalf("%s: %q, %v", name, b, err)
			}
			r, err := s4dbv1.Open(path)
			if err != nil {
				t.Fatalf("%s: source: %v", name, err)
			}
			r.Close()
		}
	})

	// A claimed build a crash cut short is replaced.
	t.Run("claimed torn converting", func(t *testing.T) {
		// Claim the files and leave a torn new file, then convert.
		path := previousVolume(t, "mixed.s4db")
		src, err := s4dbv1.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		id := sourceID(t, src)
		src.Close()
		if err := writeClaim(path+convertedMarkerSuffix, id); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path+convertingSuffix, []byte("torn"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := convertClosed(ctx, le, path); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, want)
	})

	// A verified file with its marker is installed, not rebuilt.
	t.Run("verified not installed", func(t *testing.T) {
		// Convert from the marker state, logging, and check nothing was
		// rebuilt.
		path := previousVolume(t, "mixed.s4db")
		verifiedButNotInstalled(t, path)
		logger, hook := logtest.NewNullLogger()
		if err := convertClosed(ctx, logrus.NewEntry(logger), path); err != nil {
			t.Fatal(err)
		}
		for _, e := range hook.AllEntries() {
			if e.Message == "converting volume to the current database format" {
				t.Fatal("rebuilt the verified file")
			}
		}
		checkConverted(t, path, want)
	})

	// A marker cannot authorize installation after the source changes.
	t.Run("source changed after marker", func(t *testing.T) {
		// Leave a verified copy, then replace the source with another valid
		// old-format database, as an old writer can change it between attempts.
		path := previousVolume(t, "mixed.s4db")
		verifiedButNotInstalled(t, path)
		empty, err := os.ReadFile(filepath.Join(previousFixtures, "empty.s4db"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, empty, 0o600); err != nil {
			t.Fatal(err)
		}

		// Resume must rebuild from the current source, preserving it as backup.
		if err := convertClosed(ctx, le, path); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, nil)
	})

	// Equal key counts do not establish the converted copy's contents.
	t.Run("copy changed with same count", func(t *testing.T) {
		// Change one existing value in an otherwise valid converted copy.
		path := previousVolume(t, "mixed.s4db")
		verifiedButNotInstalled(t, path)
		db, err := s4db.Open(path+convertingSuffix, s4db.Options{})
		if err != nil {
			t.Fatal(err)
		}
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			db.Close()
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte(want[0].key), []byte("changed after verification")); err != nil {
			tx.Discard()
			db.Close()
			t.Fatal(err)
		}
		if err := errors.Join(tx.Commit(ctx), db.Close()); err != nil {
			t.Fatal(err)
		}

		// Reverify every pair rather than trusting the marker's count.
		if err := convertClosed(ctx, le, path); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, want)
	})

	// The rename happens while the source lock is still held.
	t.Run("install with source locked", func(t *testing.T) {
		// Build and install without closing the old-format reader.
		path := previousVolume(t, "mixed.s4db")
		src, err := s4dbv1.OpenExclusive(path)
		if err != nil {
			t.Fatal(err)
		}
		defer src.Close()
		if _, err := isBoltFile(path); !errors.Is(err, s4db.ErrOpenInProcess) {
			t.Fatalf("format probe passed source ownership: %v", err)
		}
		if _, err := buildConverted(ctx, src, sourceID(t, src), path+convertingSuffix, path+convertedMarkerSuffix, nil); err != nil {
			t.Fatal(err)
		}
		if replaced, err := installConverted(path); err != nil || !replaced {
			t.Fatalf("install: replaced %v: %v", replaced, err)
		}

		// The retained handle still reads the complete original inode.
		n := 0
		if err := src.Stream(ctx, func(_, _ []byte) error {
			n++
			return nil
		}); err != nil || n != len(want) {
			t.Fatalf("original after install: %d keys: %v", n, err)
		}
		if err := src.Close(); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, want)
	})

	// The link made and the rename not: the link is kept.
	t.Run("linked not renamed", func(t *testing.T) {
		// Make the link by hand, then convert.
		path := previousVolume(t, "mixed.s4db")
		verifiedButNotInstalled(t, path)
		if err := os.Link(path, path+preRewriteSuffix); err != nil {
			t.Fatal(err)
		}
		if err := convertClosed(ctx, le, path); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, want)
	})

	// A torn file under a marker is rebuilt.
	t.Run("torn under marker", func(t *testing.T) {
		// Cut the verified file in half, then convert.
		path := previousVolume(t, "mixed.s4db")
		built := verifiedButNotInstalled(t, path)
		if err := os.Truncate(path+convertingSuffix, built.Size()/2); err != nil {
			t.Fatal(err)
		}
		if err := convertClosed(ctx, le, path); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, want)
	})

	// A previous-engine process that opened the file before the replace and
	// takes its locks after it is refused before it writes the backup.
	t.Run("previous opener", func(t *testing.T) {
		// Open the file as the previous engine, then convert.
		path := previousVolume(t, "mixed.s4db")
		admit, result := startPreviousOpener(t, path)
		fence, err := convertPrevious(ctx, le, path)
		if err != nil {
			t.Fatal(err)
		}
		if fence == nil {
			t.Fatal("conversion returned no fence")
		}
		defer fence.Close()

		// Let it take its locks on the file it opened, now the backup.
		admit()
		if got := result(); got != "slots full" {
			t.Fatalf("previous opener of the backup: %s", got)
		}
		if err := fence.Close(); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, want)
	})

	// A failed directory sync before the replace leaves the source in place,
	// and one after it fails the open until a later open syncs again.
	t.Run("directory sync fails", func(t *testing.T) {
		for _, after := range []bool{false, true} {
			// Fail the sync after the link, or after the rename.
			path := previousVolume(t, "mixed.s4db")
			orig, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			syncDir = func(dir string) error {
				cur, err := os.Stat(path)
				if err != nil {
					return err
				}
				_, linkErr := os.Stat(path + preRewriteSuffix)
				if linkErr == nil && os.SameFile(cur, orig) != after {
					return errors.New("injected directory sync failure")
				}
				return syncDirectory(dir)
			}
			err = convertClosed(ctx, le, path)
			syncDir = syncDirectory
			if err == nil {
				t.Fatalf("after %v: conversion passed a failed sync", after)
			}

			// After the rename the claim stays for the next open.
			_, markerErr := os.Stat(path + convertedMarkerSuffix)
			if after && markerErr != nil {
				t.Fatalf("claim dropped before the rename was durable: %v", markerErr)
			}

			// The next open syncs before it opens the file.
			synced := 0
			syncDir = func(dir string) error {
				synced++
				return syncDirectory(dir)
			}
			db, err := Open(ctx, le, path, s4db.Options{})
			syncDir = syncDirectory
			if err != nil {
				t.Fatalf("after %v: reopen: %v", after, err)
			}
			if synced == 0 {
				t.Fatalf("after %v: reopen did not sync the directory", after)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if !after {
				// The reopen converted again and holds a fence.
				continue
			}
			checkConverted(t, path, want)
		}
	})

	// A file another process holds is refused.
	t.Run("in use", func(t *testing.T) {
		// Convert while another process holds the file, then once it closes.
		path := previousVolume(t, "mixed.s4db")
		release := startPreviousHolder(t, path)
		if err := convertClosed(ctx, le, path); !errors.Is(err, ErrMigrateShared) {
			t.Fatalf("in use: %v", err)
		}
		release()
		if err := convertClosed(ctx, le, path); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, want)
	})

	// A disk without the live bytes plus a tenth free is refused.
	t.Run("no space", func(t *testing.T) {
		// Reject one byte below the bound and accept the bound.
		err := checkConvertSpace(1000, 1099)
		var noSpace *NoSpaceError
		if !errors.As(err, &noSpace) || !errors.Is(err, ErrNoSpace) || noSpace.Need != 1100 || noSpace.Free != 1099 {
			t.Fatalf("no space: %v", err)
		}
		if err := checkConvertSpace(1000, 1100); err != nil {
			t.Fatal(err)
		}
	})

	// The Volume converts an empty previous-format file when it opens.
	t.Run("volume open", func(t *testing.T) {
		// Open the Volume on the previous-format file.
		path := previousVolume(t, "empty.s4db")
		vol, err := NewVolume(ctx, le, &Config{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		defer vol.Close()
		if _, err := os.Stat(path + preRewriteSuffix); err != nil {
			t.Fatal(err)
		}
	})

	// A copy leaves the source in the previous format.
	t.Run("copy", func(t *testing.T) {
		// Copy, then check the copy as a conversion with the source linked.
		path := previousVolume(t, "mixed.s4db")
		dst := filepath.Join(t.TempDir(), "copy.s4wave")
		var keys int64
		if err := CopyPreviousFormat(ctx, path, dst, func(k, _ int64) { keys = k }); err != nil {
			t.Fatal(err)
		}
		if keys != int64(len(want)) {
			t.Fatalf("progress reached %d keys, want %d", keys, len(want))
		}
		if err := os.Link(path, dst+preRewriteSuffix); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, dst, want)

		// The source still opens in the previous format.
		r, err := s4dbv1.Open(path)
		if err != nil {
			t.Fatalf("source: %v", err)
		}
		r.Close()
	})
}

// holdPrevious opens path through the previous-format reader and holds it
// until standard input closes.
func holdPrevious(t *testing.T, path string) {
	// Open the file and hold it until standard input closes.
	r, err := s4dbv1.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	fmt.Println("open")
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		t.Fatal(err)
	}
}

// openAsPrevious opens path as the previous engine opens a file, takes its
// locks once standard input gives a line, and prints what the previous
// engine would have done: "admitted" when it claims a reader slot, after
// which it writes the file, or "slots full" when it fails before writing.
func openAsPrevious(t *testing.T, path string) {
	// Open the file, then wait for the parent.
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fmt.Println("open")
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		t.Fatal(err)
	}

	// Take and drop the writer lock, waiting, then claim a free slot
	// without waiting, at the previous engine's lock offsets.
	const lockWriter, lockSlot, slots = 1 << 40, 1<<40 + 1, 128
	if _, err := filelock.Lock(f, lockWriter, true); err != nil {
		t.Fatal(err)
	}
	if err := filelock.Unlock(f, lockWriter); err != nil {
		t.Fatal(err)
	}
	for i := range slots {
		ok, err := filelock.Lock(f, lockSlot+int64(i), false)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			fmt.Println("admitted")
			return
		}
	}
	fmt.Println("slots full")
}

// startPreviousOpener starts a process that opens path as the previous
// engine does and returns the function that lets it take its locks and the
// function that returns what it printed then.
func startPreviousOpener(t *testing.T, path string) (func(), func() string) {
	// Connect the opener's input and output.
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestConvertPrevious$", "-test.timeout=60s")
	cmd.Env = append(os.Environ(), previousOpenerEnv+"="+path)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	// Start it and wait until it has the file open.
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	out := bufio.NewReader(stdout)
	if _, err := out.ReadString('\n'); err != nil {
		t.Fatal(err)
	}

	// Admit it, then read its outcome.
	admit := func() {
		if _, err := io.WriteString(stdin, "go\n"); err != nil {
			t.Fatal(err)
		}
	}
	result := func() string {
		line, err := out.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(line)
	}
	return admit, result
}

// startPreviousHolder starts a process holding path open through the
// previous-format reader and returns the function that closes it.
func startPreviousHolder(t *testing.T, path string) func() {
	// Connect the holder's input and output.
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestConvertPrevious$", "-test.timeout=60s")
	cmd.Env = append(os.Environ(), previousHolderEnv+"="+path)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	// Start it and wait until it holds the file.
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	if _, err := bufio.NewReader(stdout).ReadString('\n'); err != nil {
		t.Fatal(err)
	}

	// Closing its input lets it exit.
	return func() {
		if err := stdin.Close(); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
}
