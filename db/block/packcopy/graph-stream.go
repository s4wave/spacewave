package packcopy

import (
	"context"
	"crypto/rand"
	"io"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// GraphStream is the per-block record of a copy, stored as objects beside the
// packs: one GraphRecords object per pack. A collector seeds from it without
// reading a pack. Objects have random names, so concurrent uploads and a
// resumed copy never collide; a block written twice has two records.
type GraphStream struct {
	// objects holds the stream.
	objects block_store_s3.ObjectStore
	// bucket is the bucket that holds it.
	bucket string
	// prefix starts each object's key.
	prefix string
}

// NewGraphStream builds a GraphStream under prefix.
func NewGraphStream(objects block_store_s3.ObjectStore, bucket, prefix string) *GraphStream {
	return &GraphStream{objects: objects, bucket: bucket, prefix: prefix}
}

// Write stores one object holding a record of each block.
func (g *GraphStream) Write(ctx context.Context, batch []*block.PutBatchEntry) error {
	// Build the records.
	records := &GraphRecords{Records: make([]*GraphRecord, len(batch))}
	for i, entry := range batch {
		records.Records[i] = &GraphRecord{Ref: entry.Ref, Size: uint64(len(entry.Data)), Refs: entry.Refs}
	}
	data, err := records.MarshalVT()
	if err != nil {
		return err
	}

	// Store them under a new name.
	return g.objects.PutObject(ctx, g.bucket, g.prefix+rand.Text(), data, "application/octet-stream")
}

// Scan calls fn with every record, one object in memory at a time.
func (g *GraphStream) Scan(ctx context.Context, fn func(*GraphRecord) error) error {
	return g.objects.ListObjects(ctx, g.bucket, g.prefix, func(key string, _ int64) error {
		// Read the object.
		body, err := g.objects.GetObject(ctx, g.bucket, key)
		if err != nil {
			return err
		}
		defer body.Close()
		data, err := io.ReadAll(body)
		if err != nil {
			return err
		}

		// Hand over its records.
		records := &GraphRecords{}
		if err := records.UnmarshalVT(data); err != nil {
			return errors.Wrap(err, key)
		}
		for _, record := range records.GetRecords() {
			if err := fn(record); err != nil {
				return err
			}
		}
		return nil
	})
}
