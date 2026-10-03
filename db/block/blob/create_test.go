package blob

import (
	"bytes"
	"context"
	"io"
	"slices"
	"testing"

	"github.com/aperturerobotics/util/prng"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

type blobMetricRecorder struct {
	metrics []Metric
}

func (r *blobMetricRecorder) RecordBlobMetric(metric Metric) {
	r.metrics = append(r.metrics, metric)
}

// TestBuildBlobWithBytes tests building a Blob from a byte slice.
func TestBuildBlobWithBytes(t *testing.T) {
	// Prepare the context and logger for the byte-slice blob fixture.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for building a byte-slice blob.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty cursor for the blob transaction.
	cs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Choose the byte payload used for the blob round trip.
	data := []byte("hello world 1234")

	// Build the byte-slice blob in a new transaction.
	btx, bcs := cs.BuildTransactionAtRef(nil, nil)
	_, err = BuildBlobWithBytes(ctx, data, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Persist the byte-slice blob and report its root reference.
	bref, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("blob written to %s", bref.MarshalString())

	// Reload the persisted blob bytes through a fresh transaction.
	cs.SetRootRef(bref)
	_, bcs = cs.BuildTransaction(nil)
	fetched, err := FetchToBytes(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the stored blob bytes match the original payload.
	if !bytes.Equal(fetched, data) {
		t.Fatalf("mismatch of fetched data: %#v != expected %#v", fetched, data)
	}

	// Reload the blob metadata and validate its stored graph.
	_, bcs = cs.BuildTransaction(nil)
	b1, err := UnmarshalBlob(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := b1.ValidateFull(ctx, bcs); err != nil {
		t.Fatal(err.Error())
	}
}

// TestBuildBlobWithReader tests building a Blob from a reader w/o known size.
func TestBuildBlobWithReader(t *testing.T) {
	// Prepare metrics recording and logging for unknown-size blob builds.
	ctx := context.Background()
	recorder := &blobMetricRecorder{}
	ctx = WithMetricsRecorder(ctx, recorder)
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for unknown-size blob builds.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty cursor for the reader-backed blob fixture.
	cs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Provide repeatable input bytes for raw and chunked reader builds.
	buildReader := func() io.Reader {
		return prng.BuildSeededReader([]byte("test-chunk-blob"))
	}

	// Test with data less than high water mark
	_, bcs := cs.BuildTransactionAtRef(nil, nil)
	builtBlob, err := BuildBlobWithReader(
		ctx,
		io.LimitReader(buildReader(), DefRawHighWaterMark-2),
		bcs,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the short reader builds a raw blob.
	if builtBlob.GetBlobType() != BlobType_BlobType_RAW {
		t.Fatalf("Expected raw blob but got %v", builtBlob.GetBlobType().String())
	}

	// Test with data more than high water mark
	chunkedData := make([]byte, DefRawHighWaterMark*2)
	if _, err := io.ReadAtLeast(buildReader(), chunkedData, len(chunkedData)); err != nil {
		t.Fatal(err.Error())
	}

	// Build a chunked blob from reader input larger than the raw limit.
	btx, bcs := cs.BuildTransactionAtRef(nil, nil)
	builtBlob, err = BuildBlobWithReader(
		ctx,
		io.LimitReader(buildReader(), int64(len(chunkedData))),
		bcs,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the large reader builds a chunked blob and records both build paths.
	if builtBlob.GetBlobType() != BlobType_BlobType_CHUNKED {
		t.Fatalf("Expected chunked blob but got %v", builtBlob.GetBlobType().String())
	}
	if !slices.ContainsFunc(recorder.metrics, func(metric Metric) bool {
		return metric.Stage == "raw" && metric.InputBytes == DefRawHighWaterMark-2
	}) {
		t.Fatalf("missing raw blob metric: %#v", recorder.metrics)
	}
	if !slices.ContainsFunc(recorder.metrics, func(metric Metric) bool {
		return metric.Stage == "chunked"
	}) {
		t.Fatalf("missing chunked blob metric: %#v", recorder.metrics)
	}
	if !slices.ContainsFunc(recorder.metrics, func(metric Metric) bool {
		return metric.Stage == "chunk-direct-put" && metric.DirectPut
	}) {
		t.Fatalf("missing direct put chunk metric: %#v", recorder.metrics)
	}

	// Persist the reader-built chunked blob for a fresh read transaction.
	ref, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Reload the persisted chunked blob and validate its metadata.
	cs.SetRootRef(ref)
	_, bcs = cs.BuildTransaction(nil)
	b1, err := UnmarshalBlob(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := b1.ValidateFull(ctx, bcs); err != nil {
		t.Fatal(err.Error())
	}

	// Read the persisted chunked blob through its reader.
	rdr, err := NewReader(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	readData, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the stored chunked bytes match the generated payload.
	if !bytes.Equal(readData, chunkedData) {
		t.Fatal("mismatch of read data from chunked test")
	}
}

func TestBuildBlobWithReaderFallbackCopyIsAccounted(t *testing.T) {
	// Record metrics for an unknown-size blob built without a backing store.
	ctx := context.Background()
	recorder := &blobMetricRecorder{}
	ctx = WithMetricsRecorder(ctx, recorder)

	// Build the reader-backed blob through an ephemeral transaction.
	_, bcs := block.NewTransaction(nil, nil, nil, nil)
	data := []byte("chunk fallback payload")
	_, err := BuildBlobWithReader(
		ctx,
		bytes.NewReader(data),
		bcs,
		&BuildBlobOpts{RawHighWaterMark: 1},
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify metrics account for the fallback chunk data copy.
	if !slices.ContainsFunc(recorder.metrics, func(metric Metric) bool {
		return metric.Stage == "chunk-fallback-copy" && metric.ChunkBytes > 0 && !metric.DirectPut
	}) {
		t.Fatalf("missing accounted fallback-copy metric: %#v", recorder.metrics)
	}
}
