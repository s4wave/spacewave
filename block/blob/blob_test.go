package blob

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/util/prng"
)

// buildMockRawBlob builds a new mock raw blob.
func buildMockRawBlob() *Blob {
	testBuf := []byte("test-raw-blob")
	return &Blob{
		BlobType:  BlobType_BlobType_RAW,
		TotalSize: uint64(len(testBuf)),
		RawData:   testBuf,
	}
}

// buildMockChunkedBlob builds a new mock chunked blob
func buildMockChunkedBlob(bcs *block.Cursor) (*Blob, error) {
	// generate 100Mb of data
	rd := prng.BuildSeededRand([]byte("test-chunk-blob"))
	data := make([]byte, 100e6)
	_, err := rd.Read(data)
	if err != nil {
		return nil, err
	}
	return BuildBlob(
		context.Background(),
		int64(len(data)),
		bytes.NewReader(data),
		bcs,
		&BuildBlobOpts{
			RawHighWaterMark: 1,
			// pre-compute polynomial to save time
			ChunkerArgs: &ChunkerArgs{
				ChunkerType: ChunkerType_ChunkerType_RABIN,
				RabinArgs: &RabinArgs{
					Pol: 13388372929173625,
				},
			},
		},
	)
}
