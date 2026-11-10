//go:build !js && !wasip1 && !windows

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

func testKey(data []byte) []byte {
	sum := sha256.Sum256(data)
	return append(append([]byte{}, keyPrefix...), sum[:]...)
}

func encRefs(t *testing.T, refs []*block.BlockRef) []byte {
	var out []byte
	for _, r := range refs {
		b, err := r.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		out = binary.AppendUvarint(out, uint64(len(b)))
		out = append(out, b...)
	}
	if out == nil {
		out = []byte{}
	}
	return out
}

// TestWriteResume writes two batches, cuts the files as a crash mid-batch
// would leave them, resumes, and checks the frames decode and verify.
func TestWriteResume(t *testing.T) {
	dir := t.TempDir()
	d := [][]byte{[]byte("a"), []byte("bb"), []byte("ccc"), []byte("dddd")}
	keys := make([][]byte, len(d))
	for i := range d {
		keys[i] = testKey(d[i])
	}
	child := &block.BlockRef{Hash: hash.NewHash(hash.HashType_HashType_SHA256, sha256.New().Sum(nil))}
	rows := map[string]row{
		string(keys[0]): {data: d[0], refs: encRefs(t, []*block.BlockRef{child}), known: true},
		string(keys[1]): {data: d[1], refs: nil, known: false},
		string(keys[3]): {data: []byte("wrong"), refs: []byte{}, known: true},
	}

	o, err := openOutput(dir, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := o.write(0, keys[:2], rows); err != nil {
		t.Fatal(err)
	}
	// A second batch whose ledger line never lands: frames written, no line.
	if _, err := o.blocks.Write([]byte("partial")); err != nil {
		t.Fatal(err)
	}
	o.close()

	o, err = openOutput(dir, "t")
	if err != nil {
		t.Fatal(err)
	}
	if o.next != 1 || o.found != 2 || o.missed != 0 {
		t.Fatalf("resume next %d found %d missed %d", o.next, o.found, o.missed)
	}
	if err := o.write(1, keys[2:], rows); err != nil {
		t.Fatal(err)
	}
	o.close()

	misses, _ := os.ReadFile(filepath.Join(dir, "t.misses"))
	bad, _ := os.ReadFile(filepath.Join(dir, "t.hashbad"))
	if bytes.Count(misses, []byte("\n")) != 1 || bytes.Count(bad, []byte("\n")) != 1 {
		t.Fatalf("misses %q bad %q", misses, bad)
	}

	// Decode the frames as block-export frames.
	b, _ := os.ReadFile(filepath.Join(dir, "t.blocks"))
	var n int
	for len(b) > 0 {
		kl, w := binary.Uvarint(b)
		b = b[w:]
		key := b[:kl]
		b = b[kl:]
		known, w := binary.Uvarint(b)
		b = b[w:]
		ol, w := binary.Uvarint(b)
		b = b[w:]
		stored, err := block.DecodeBlockObject(b[:ol])
		if err != nil {
			t.Fatal(err)
		}
		b = b[ol:]
		sum := sha256.Sum256(stored.Data)
		if !bytes.Equal(sum[:], key[4:]) {
			t.Fatalf("frame %d hash", n)
		}
		if (known == 1) != stored.RefsKnown {
			t.Fatalf("frame %d known %d refsKnown %v", n, known, stored.RefsKnown)
		}
		if n == 0 && (len(stored.Refs) != 1 || !stored.Refs[0].EqualVT(child)) {
			t.Fatalf("frame 0 refs %v", stored.Refs)
		}
		n++
	}
	if n != 2 {
		t.Fatalf("frames %d", n)
	}
}

// TestReadEnvNoEcho checks a malformed env file's error names no text.
func TestReadEnvNoEcho(t *testing.T) {
	p := filepath.Join(t.TempDir(), "e")
	if err := os.WriteFile(p, []byte("HYPERLINE_WORLD_SQL_HOST=h\nSECRETVALUE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readEnv(p)
	if err == nil || bytes.Contains([]byte(err.Error()), []byte("SECRET")) {
		t.Fatalf("err %v", err)
	}
}
