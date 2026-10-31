package s4dbv1

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
)

// row is one streamed pair as the fixture's expected files record it: the
// key, the value length and the value's SHA-256.
type row struct {
	key  string
	n    int
	hash string
}

// readRows parses an expected file.
func readRows(t *testing.T, path string) []row {
	// Read the file and parse each line.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []row
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		f := strings.Split(line, "\t")
		var r row
		r.key, r.hash = f[0], f[2]
		fmt.Sscan(f[1], &r.n)
		rows = append(rows, r)
	}
	return rows
}

// copyFixture copies a testdata file into a temporary directory, since Open
// takes locks and the torn tests write.
func copyFixture(t *testing.T, name string) string {
	// Read the fixture and write the copy.
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// streamRows opens path and streams it into rows.
func streamRows(t *testing.T, path string) []row {
	// Open the file and collect each pair, checking the order.
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	// Collect each pair, checking the order.
	var rows []row
	var last []byte
	err = r.Stream(context.Background(), func(key, value []byte) error {
		// Check the order and record the pair.
		if bytes.Compare(key, last) <= 0 && last != nil {
			return errors.Errorf("key %q not above %q", key, last)
		}
		last = bytes.Clone(key)
		sum := sha256.Sum256(value)
		rows = append(rows, row{key: string(key), n: len(value), hash: hex.EncodeToString(sum[:])})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// expectRows fails unless got matches want row for row.
func expectRows(t *testing.T, got, want []row) {
	if len(got) != len(want) {
		t.Fatalf("streamed %d pairs, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pair %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestStream streams the mixed fixture and compares it with the expected
// rows, then streams the empty fixture.
func TestStream(t *testing.T) {
	// The mixed fixture: tree, log, extents and a link.
	path := copyFixture(t, "mixed.s4db")
	expectRows(t, streamRows(t, path), readRows(t, filepath.Join("testdata", "mixed.expected.tsv")))
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.LiveBytes() <= 0 {
		t.Fatalf("live bytes %d", r.LiveBytes())
	}
	t.Logf("mixed: live estimate %d bytes, tree pages %d", r.LiveBytes(), r.sb.treePages)
	r.Close()

	// The empty fixture streams nothing.
	if rows := streamRows(t, copyFixture(t, "empty.s4db")); len(rows) != 0 {
		t.Fatalf("empty file streamed %d pairs", len(rows))
	}
}

// TestStreamTorn cuts the mixed fixture's last commit two ways, by zeroing
// its header and by zeroing one of its extents, and expects the state before
// it both times, as the previous engine's recovery would have opened it.
func TestStreamTorn(t *testing.T) {
	want := readRows(t, filepath.Join("testdata", "mixed.before-last.tsv"))
	for _, way := range []string{"header", "extent"} {
		// Find the last commit.
		path := copyFixture(t, "mixed.s4db")
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		sb, err := readSuper(f)
		if err != nil {
			t.Fatal(err)
		}
		lr := newLogReader(f, sb)
		var last *record
		var lastPos uint64
		for {
			pos := lr.pos
			rec, ok := lr.next()
			if !ok {
				break
			}
			if rec.kind == kindCommit {
				last, lastPos = rec, pos
			}
		}
		if last == nil {
			t.Fatal("no commit in the log")
		}

		// Tear it.
		at := int64(lastPos)
		if way == "extent" {
			at = -1
			for _, o := range last.ops {
				if o.val.isRef {
					at = int64(o.val.ref.off)
					break
				}
			}
			if at < 0 {
				t.Fatal("last commit has no extent")
			}
		}
		if _, err := f.WriteAt(make([]byte, 8), at); err != nil {
			t.Fatal(err)
		}
		f.Close()
		expectRows(t, streamRows(t, path), want)
	}
}
