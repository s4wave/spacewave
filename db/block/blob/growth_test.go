package blob

import (
	"bytes"
	"io"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestAppendAfterZeroGrowth keeps implicit zeros before new bytes through both tail paths.
func TestAppendAfterZeroGrowth(t *testing.T) {
	// Exercise fitting and rechunked tails under both persisted algorithms.
	for _, kind := range []ChunkerType{ChunkerType_ChunkerType_JC, ChunkerType_ChunkerType_RABIN} {
		for _, growth := range []int{17, 600} {
			t.Run(kind.String()+"/"+strconv.Itoa(growth), func(t *testing.T) {
				// Acquire the real storage testbed and register its controller cleanup.
				ctx := t.Context()
				tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(tb.Release)
				oc, err := tb.BuildEmptyCursor(ctx)
				if err != nil {
					t.Fatal(err)
				}

				// Build stored bytes with small boundaries so both append paths are reachable.
				opts := &BuildBlobOpts{
					RawHighWaterMark: 1,
					ChunkerArgs: &ChunkerArgs{
						ChunkerType: kind,
						JcArgs: &JcArgs{
							ChunkingMinSize: 64, ChunkingTargetSize: 128, ChunkingMaxSize: 256,
						},
						RabinArgs: &RabinArgs{ChunkingMinSize: 64, ChunkingMaxSize: 256},
					},
				}
				original := bytes.Repeat([]byte("original-data/"), 30)
				btx, cursor := oc.BuildTransactionAtRef(nil, nil)
				blob, err := BuildBlob(ctx, int64(len(original)), bytes.NewReader(original), cursor, opts)
				if err != nil {
					t.Fatal(err)
				}

				// Publish logical growth without materializing the zero suffix.
				if err := blob.Truncate(ctx, cursor, opts, int64(len(original)+growth)); err != nil {
					t.Fatal(err)
				}
				ref, _, err := btx.Write(ctx, true)
				if err != nil {
					t.Fatal(err)
				}

				// Reopen the grown value before appending through its persisted tail configuration.
				btx, cursor = oc.BuildTransactionAtRef(nil, ref)
				blob, err = UnmarshalBlob(ctx, cursor)
				if err != nil {
					t.Fatal(err)
				}
				appended := []byte("appended")
				if err := blob.AppendData(ctx, int64(len(appended)), bytes.NewReader(appended), cursor, opts); err != nil {
					t.Fatal(err)
				}
				ref, _, err = btx.Write(ctx, true)
				if err != nil {
					t.Fatal(err)
				}

				// Open the committed root through the normal Blob reader.
				_, cursor = oc.BuildTransactionAtRef(nil, ref)
				rdr, err := NewReader(ctx, cursor)
				if err != nil {
					t.Fatal(err)
				}
				defer rdr.Close()

				// Compare complete content, including zeros between old and appended bytes.
				want := make([]byte, len(original)+growth+len(appended))
				copy(want, original)
				copy(want[len(original)+growth:], appended)
				got, err := io.ReadAll(io.LimitReader(rdr, int64(len(want)+1)))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("append after %d zero bytes: got %d bytes, want %d with the gap intact", growth, len(got), len(want))
				}
			})
		}
	}
}
