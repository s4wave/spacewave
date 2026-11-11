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
	"github.com/s4wave/spacewave/db/s4db"
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
	for line := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
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

	// Duplicate and hard-link opens must not drop this reader's source lock.
	alias := path + ".alias"
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{path, alias} {
		other, err := Open(name)
		if other != nil {
			other.Close()
		}
		if !errors.Is(err, ErrInUse) {
			t.Fatalf("duplicate source %s: %v", name, err)
		}
	}

	// A new-engine format probe must not open and close the locked old inode.
	if db, err := s4db.Open(alias, s4db.Options{}); !errors.Is(err, s4db.ErrOpenInProcess) {
		if db != nil {
			db.Close()
		}
		t.Fatalf("engine probe passed source ownership: %v", err)
	}

	// Keep the original reader usable after every refused probe.
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

// TestGetReadOnly looks up every key of the mixed fixture, the keys its last
// commit deleted and keys it never held, through a copy no one may write, and
// checks the copy's bytes and modification time are unchanged after.
func TestGetReadOnly(t *testing.T) {
	// Copy the fixture and take away write permission.
	path := copyFixture(t, "mixed.s4db")
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// Every key the file holds reads back as the stream has it.
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want := readRows(t, filepath.Join("testdata", "mixed.expected.tsv"))
	held := make(map[string]bool, len(want))
	for _, w := range want {
		held[w.key] = true
		v, ok, err := r.Get([]byte(w.key))
		if err != nil || !ok {
			t.Fatalf("get %q: found %v, %v", w.key, ok, err)
		}
		sum := sha256.Sum256(v)
		if got := (row{key: w.key, n: len(v), hash: hex.EncodeToString(sum[:])}); got != w {
			t.Fatalf("get %q: got %+v, want %+v", w.key, got, w)
		}
	}

	// Keys the last commit deleted and keys never written are missing.
	missing := []string{"", "\x00", "l00000\x00", "zzzz"}
	for _, b := range readRows(t, filepath.Join("testdata", "mixed.before-last.tsv")) {
		if !held[b.key] {
			missing = append(missing, b.key)
		}
	}
	for _, k := range missing {
		if v, ok, err := r.Get([]byte(k)); err != nil || ok {
			t.Fatalf("get %q: found %v with %d bytes, %v", k, ok, len(v), err)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	// The file is as it was.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatalf("file changed: mtime %v to %v", beforeInfo.ModTime(), afterInfo.ModTime())
	}
	t.Logf("mixed: %d keys found, %d missing", len(want), len(missing))
}
