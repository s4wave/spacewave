//go:build !js && !wasip1

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
	"github.com/s4wave/spacewave/db/s4db/s4dbv1"
	"github.com/sirupsen/logrus"
)

// previousFixtures holds the files the previous engine wrote.
const previousFixtures = "../../s4db/s4dbv1/testdata"

// previousHolderEnv names the previous-format file a test process holds open
// until its standard input closes.
const previousHolderEnv = "SPACEWAVE_S4DBV1_HOLDER"

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

// verifiedButNotInstalled runs the conversion through the marker and stops
// before the install, as a crash there would, returning the new file's
// identity.
func verifiedButNotInstalled(t *testing.T, path string) os.FileInfo {
	// Build and verify.
	ctx := context.Background()
	src, err := s4dbv1.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := buildConverted(ctx, src, path+convertingSuffix, path+convertedMarkerSuffix, nil); err != nil {
		t.Fatal(err)
	}

	// Identify the new file.
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
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	want := expectedRows(t)

	// A fresh conversion, and the file opens without converting again.
	t.Run("fresh", func(t *testing.T) {
		// Convert twice.
		path := previousVolume(t, "mixed.s4db")
		if err := convertPrevious(ctx, le, path); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, want)
		if err := convertPrevious(ctx, le, path); err != nil {
			t.Fatal(err)
		}
	})

	// A stale unverified file from a crash before the marker is replaced.
	t.Run("stale converting", func(t *testing.T) {
		// Leave a torn new file, then convert.
		path := previousVolume(t, "mixed.s4db")
		if err := os.WriteFile(path+convertingSuffix, []byte("torn"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := convertPrevious(ctx, le, path); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, want)
	})

	// A verified file with its marker is installed, not rebuilt.
	t.Run("verified not installed", func(t *testing.T) {
		// Convert from the marker state and check the file was kept.
		path := previousVolume(t, "mixed.s4db")
		built := verifiedButNotInstalled(t, path)
		if err := convertPrevious(ctx, le, path); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, want)
		fi, err := os.Stat(path)
		if err != nil || !os.SameFile(built, fi) {
			t.Fatalf("installed file is not the verified one: %v", err)
		}
	})

	// The link made and the rename not: the link is kept.
	t.Run("linked not renamed", func(t *testing.T) {
		// Make the link by hand, then convert.
		path := previousVolume(t, "mixed.s4db")
		verifiedButNotInstalled(t, path)
		if err := os.Link(path, path+preRewriteSuffix); err != nil {
			t.Fatal(err)
		}
		if err := convertPrevious(ctx, le, path); err != nil {
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
		if err := convertPrevious(ctx, le, path); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, want)
	})

	// A file another process holds is refused.
	t.Run("in use", func(t *testing.T) {
		// Convert while another process holds the file, then once it closes.
		path := previousVolume(t, "mixed.s4db")
		release := startPreviousHolder(t, path)
		if err := convertPrevious(ctx, le, path); !errors.Is(err, ErrMigrateShared) {
			t.Fatalf("in use: %v", err)
		}
		release()
		if err := convertPrevious(ctx, le, path); err != nil {
			t.Fatal(err)
		}
		checkConverted(t, path, want)
	})

	// A disk without the live bytes plus a tenth free is refused.
	t.Run("no space", func(t *testing.T) {
		// One byte short refuses; the bound passes.
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
		if _, err := s4dbv1.Open(path); err != nil {
			t.Fatalf("source: %v", err)
		}
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
