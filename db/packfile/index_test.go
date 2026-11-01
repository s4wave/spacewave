package packfile

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
)

// TestReadIndexTail reads index tails that fit the first read and tails that
// need the missing prefix read backwards, and checks each read takes only
// bytes not read before.
func TestReadIndexTail(t *testing.T) {
	for _, tc := range []struct {
		name    string
		keys    int
		keySize int
	}{
		{"fits", 100, 32},
		{"backwards", 1000, 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Build keys of the case's size.
			keys := make([][]byte, tc.keys)
			for i := range keys {
				keys[i] = fmt.Appendf(nil, "%0*d", tc.keySize, i)
			}

			// Write a pack whose first value pushes the tail away from the start.
			var buf bytes.Buffer
			err := kvfile.Write(&buf, keys, func(wr io.Writer, key []byte) (uint64, error) {
				value := key
				if bytes.Equal(key, keys[0]) {
					value = make([]byte, 2<<20)
				}
				n, err := wr.Write(value)
				return uint64(n), err
			})
			if err != nil {
				t.Fatal(err)
			}

			// Read the tail the whole-file way for comparison.
			pack := buf.Bytes()
			size := uint64(len(pack))
			_, want, err := kvfile.ReadIndexTail(bytes.NewReader(pack), size)
			if err != nil {
				t.Fatal(err)
			}

			// Read it backwards and record each range.
			type readRange struct{ off, end int64 }
			var reads []readRange
			got, err := ReadIndexTail(t.Context(), size, func(_ context.Context, off int64, length int) ([]byte, error) {
				reads = append(reads, readRange{off, off + int64(length)})
				return pack[off : off+int64(length)], nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("tail = %d bytes, want %d", len(got), len(want))
			}

			// The first read ends the pack; each later read ends where the last began.
			if reads[0].end != int64(size) || reads[0].end-reads[0].off != IndexTailFirstRead {
				t.Fatalf("first read = %+v, want the last %d bytes", reads[0], IndexTailFirstRead)
			}
			for i := 1; i < len(reads); i++ {
				if reads[i].end != reads[i-1].off {
					t.Fatalf("read %d = %+v, want it to end at %d", i, reads[i], reads[i-1].off)
				}
			}
			first := reads[len(reads)-1].off
			if over := int64(size) - int64(len(want)) - first; len(want) > IndexTailFirstRead && (over < 0 || over > maxIndexEntrySize) {
				t.Fatalf("read %d bytes below the tail, want at most %d", over, maxIndexEntrySize)
			}
			if len(reads) > 3 {
				t.Fatalf("%d reads, want at most 3", len(reads))
			}
			if tc.name == "backwards" && len(reads) == 1 {
				t.Fatalf("tail of %d bytes fit the first read; grow the test keys", len(want))
			}
		})
	}
}
