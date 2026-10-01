package blob

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/db/testbed"
)

// TestBlobTransformToChunked preserves bytes through explicit conversion and growth.
func TestBlobTransformToChunked(t *testing.T) {
	for _, grow := range []bool{false, true} {
		// Give each conversion its own real volume and transaction lifecycle.
		name := "explicit"
		if grow {
			name = "growth"
		}
		testbed.RunSubtest(t, name, func(t *testing.T, tb *testbed.Testbed) {
			// Acquire the volume cursor used for every transaction in this conversion.
			ctx := t.Context()
			cursor, err := tb.BuildEmptyCursor(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer cursor.Release()

			// Commit raw bytes that the conversion must preserve.
			btx, bcs := cursor.BuildTransaction(nil)
			data := bytes.Repeat([]byte("raw content"), 32)
			value := NewRawBlob(data)
			bcs.SetBlock(value, true)
			ref, _, err := btx.Write(ctx, true)
			if err != nil {
				t.Fatal(err)
			}

			// Reopen the raw value so conversion must also mark the root dirty.
			btx, bcs = cursor.BuildTransactionAtRef(nil, ref)
			value, err = UnmarshalBlob(ctx, bcs)
			if err != nil {
				t.Fatal(err)
			}

			// Convert explicitly or grow the value beyond its raw high-water mark.
			want := bytes.Clone(data)
			if grow {
				want = append(want, make([]byte, 17)...)
				err = value.Truncate(ctx, bcs, &BuildBlobOpts{RawHighWaterMark: uint64(len(data))}, int64(len(want)))
			} else {
				err = value.TransformToChunked(ctx, bcs, nil)
			}
			if err != nil {
				t.Fatal(err)
			}

			// Require a valid chunk index with no retained inline raw bytes.
			if value.GetBlobType() != BlobType_BlobType_CHUNKED {
				t.Fatalf("blob type = %s, want chunked", value.GetBlobType())
			}
			if err := value.Validate(); err != nil {
				t.Fatal(err)
			}
			ref, _, err = btx.Write(ctx, true)
			if err != nil {
				t.Fatal(err)
			}

			// Reopen the published root and read every original and zero-filled byte.
			_, bcs = cursor.BuildTransactionAtRef(nil, ref)
			value, err = UnmarshalBlob(ctx, bcs)
			if err != nil {
				t.Fatal(err)
			}
			if value.GetBlobType() != BlobType_BlobType_CHUNKED {
				t.Fatalf("published blob type = %s, want chunked", value.GetBlobType())
			}
			got, err := FetchToBytes(ctx, bcs)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("converted bytes = %x, want %x", got, want)
			}
		})
	}
}
