package unixfs_block

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/block/file"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

func TestUnixFSBlockRangeMetricWriteAtWriteBlobTruncate(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(NewFSNode(NodeType_NodeType_DIRECTORY, 0, nil), true)
	root, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}
	if mkErr := Mknod(root, [][]string{{"metric-writeat"}}, NodeType_NodeType_FILE, 0, nil); mkErr != nil {
		t.Fatal(mkErr.Error())
	}
	if mkErr := Mknod(root, [][]string{{"metric-writeblob"}}, NodeType_NodeType_FILE, 0, nil); mkErr != nil {
		t.Fatal(mkErr.Error())
	}
	first := bytes.Repeat([]byte("unixfs-writeat-"), 96)
	second := bytes.Repeat([]byte("blob-append-"), 48)
	truncateTail := make([]byte, 16)
	expected := append(append(append([]byte(nil), first...), second...), truncateTail...)

	if writeErr := WriteAt(ctx, root, nil, []string{"metric-writeat"}, 0, int64(len(first)), bytes.NewReader(first), nil); writeErr != nil {
		t.Fatal(writeErr.Error())
	}
	if writeErr := WriteAt(ctx, root, nil, []string{"metric-writeat"}, int64(len(first)), int64(len(second)), bytes.NewReader(second), nil); writeErr != nil {
		t.Fatal(writeErr.Error())
	}
	if truncateErr := TruncateFile(ctx, root, []string{"metric-writeat"}, int64(len(expected)), nil); truncateErr != nil {
		t.Fatal(truncateErr.Error())
	}

	if writeErr := WriteAt(ctx, root, nil, []string{"metric-writeblob"}, 0, int64(len(first)), bytes.NewReader(first), nil); writeErr != nil {
		t.Fatal(writeErr.Error())
	}
	blobBtx, blobBcs := oc.BuildTransaction(nil)
	_, err = blob.BuildBlob(ctx, int64(len(second)), bytes.NewReader(second), blobBcs, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	blobRef, _, err := blobBtx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if writeErr := WriteBlob(ctx, root, []string{"metric-writeblob"}, int64(len(first)), blobRef, true, false, nil); writeErr != nil {
		t.Fatal(writeErr.Error())
	}
	if truncateErr := TruncateFile(ctx, root, []string{"metric-writeblob"}, int64(len(expected)), nil); truncateErr != nil {
		t.Fatal(truncateErr.Error())
	}

	rootWriteStarted := time.Now()
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	rootWriteLatency := time.Since(rootWriteStarted)
	root, err = NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	readMetricFile := func(name string) ([]byte, int, int, time.Duration) {
		child, _, err := root.LookupFollowDirent(name)
		if err != nil {
			t.Fatal(err.Error())
		}
		fh, err := child.BuildFileHandle(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer fh.Close()
		readStarted := time.Now()
		out, err := io.ReadAll(fh)
		readLatency := time.Since(readStarted)
		if err != nil {
			t.Fatal(err.Error())
		}
		rootFile, err := block.UnmarshalBlock[*file.File](ctx, fh.GetCursor(), file.NewFileBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		metadataBytes, err := rootFile.MarshalBlock()
		if err != nil {
			t.Fatal(err.Error())
		}
		return out, len(rootFile.GetRanges()), len(metadataBytes), readLatency
	}
	writeAtOut, writeAtRanges, writeAtMetadataBytes, writeAtReadLatency := readMetricFile("metric-writeat")
	if !bytes.Equal(writeAtOut, expected) {
		t.Fatal("unixfs WriteAt append metric readback mismatch")
	}
	writeBlobOut, writeBlobRanges, writeBlobMetadataBytes, writeBlobReadLatency := readMetricFile("metric-writeblob")
	if !bytes.Equal(writeBlobOut, expected) {
		t.Fatal("unixfs WriteBlob append metric readback mismatch")
	}
	if writeAtRanges != writeBlobRanges {
		t.Fatalf("unixfs append parity range mismatch: writeat=%d writeblob=%d", writeAtRanges, writeBlobRanges)
	}
	t.Logf("metric workload=unixfs-write-boundary file_class=unixfs-block chunk_class=blob write_at_bytes=%d write_at_append_bytes=%d write_blob_bytes=%d truncate_size=%d range_count=%d write_at_range_count=%d write_blob_range_count=%d read_latency_ns=%d serialized_metadata_bytes=%d root_write_latency_ns=%d metadata_rewrite_bytes_per_append=%d metadata_rewrite_bytes_per_publish=%d", len(first), len(second), len(second), len(expected), writeAtRanges, writeAtRanges, writeBlobRanges, writeAtReadLatency.Nanoseconds()+writeBlobReadLatency.Nanoseconds(), writeAtMetadataBytes+writeBlobMetadataBytes, rootWriteLatency.Nanoseconds(), writeAtMetadataBytes, writeBlobMetadataBytes)
}

func TestUnixFSBlockRangeMetricRandomOverwrite(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(NewFSNode(NodeType_NodeType_DIRECTORY, 0, nil), true)
	root, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}
	if mkErr := Mknod(root, [][]string{{"metric-random-overwrite"}}, NodeType_NodeType_FILE, 0, nil); mkErr != nil {
		t.Fatal(mkErr.Error())
	}

	body := bytes.Repeat([]byte("unixfs-random-overwrite-base-"), 128)
	expected := append([]byte(nil), body...)
	if writeErr := WriteAt(ctx, root, nil, []string{"metric-random-overwrite"}, 0, int64(len(body)), bytes.NewReader(body), nil); writeErr != nil {
		t.Fatal(writeErr.Error())
	}
	writes := []struct {
		offset int
		data   []byte
	}{
		{96, bytes.Repeat([]byte("a"), 64)},
		{96, bytes.Repeat([]byte("b"), 64)},
		{220, bytes.Repeat([]byte("c"), 48)},
		{300, bytes.Repeat([]byte("d"), 24)},
	}
	for _, write := range writes {
		copy(expected[write.offset:], write.data)
		if writeErr := WriteAt(ctx, root, nil, []string{"metric-random-overwrite"}, int64(write.offset), int64(len(write.data)), bytes.NewReader(write.data), nil); writeErr != nil {
			t.Fatal(writeErr.Error())
		}
	}

	rootWriteStarted := time.Now()
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	rootWriteLatency := time.Since(rootWriteStarted)
	root, err = NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}
	child, _, err := root.LookupFollowDirent("metric-random-overwrite")
	if err != nil {
		t.Fatal(err.Error())
	}
	fh, err := child.BuildFileHandle(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fh.Close()
	readStarted := time.Now()
	out, err := io.ReadAll(fh)
	readLatency := time.Since(readStarted)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(out, expected) {
		diff := firstByteDiff(out, expected)
		t.Fatalf("unixfs random overwrite readback mismatch: got_len=%d want_len=%d first_diff=%d got_window=%q want_window=%q", len(out), len(expected), diff, byteWindow(out, diff), byteWindow(expected, diff))
	}
	rootFile, err := block.UnmarshalBlock[*file.File](ctx, fh.GetCursor(), file.NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	metadataBytes, err := rootFile.MarshalBlock()
	if err != nil {
		t.Fatal(err.Error())
	}
	occluded := countFullyOccludedRanges(rootFile.GetRanges())
	overlapDepth := maxOverlapDepth(rootFile.GetRanges())
	lookupScan := lookupScanLength(rootFile.GetRanges(), 128)
	uncompactedRangeCount := 1 + len(writes)
	if len(rootFile.GetRanges()) <= 1 || len(rootFile.GetRanges()) >= uncompactedRangeCount || occluded != 0 || overlapDepth <= 1 {
		t.Fatalf("unixfs random overwrite workload did not preserve compacted range pressure: ranges=%d uncompacted_ranges=%d occluded=%d overlap_depth=%d", len(rootFile.GetRanges()), uncompactedRangeCount, occluded, overlapDepth)
	}
	t.Logf("metric workload=unixfs-random-overwrite file_class=unixfs-block chunk_class=file range_count=%d uncompacted_range_count=%d fully_occluded_range_count=%d stale_reachable_refs=%d overlap_depth=%d lookup_scan_length=%d logical_bytes=%d read_latency_ns=%d serialized_metadata_bytes=%d root_write_latency_ns=%d metadata_rewrite_bytes_per_append=%d metadata_rewrite_bytes_per_publish=%d", len(rootFile.GetRanges()), uncompactedRangeCount, occluded, occluded, overlapDepth, lookupScan, rootFile.GetTotalSize(), readLatency.Nanoseconds(), len(metadataBytes), rootWriteLatency.Nanoseconds(), len(metadataBytes), len(metadataBytes))
}

func firstByteDiff(got, want []byte) int {
	limit := min(len(got), len(want))
	for i := range limit {
		if got[i] != want[i] {
			return i
		}
	}
	if len(got) != len(want) {
		return limit
	}
	return -1
}

func byteWindow(data []byte, center int) []byte {
	if center < 0 {
		return nil
	}
	start := max(center-8, 0)
	end := min(center+24, len(data))
	return data[start:end]
}

func countFullyOccludedRanges(ranges []*file.Range) int {
	var count int
	for i, rng := range ranges {
		if rangeFullyOccluded(i, ranges, rng) {
			count++
		}
	}
	return count
}

func rangeFullyOccluded(idx int, ranges []*file.Range, rng *file.Range) bool {
	start := rng.GetStart()
	end := start + rng.GetLength()
	for pos := start; pos < end; pos++ {
		covered := false
		for j, other := range ranges {
			if j == idx || other.GetNonce() <= rng.GetNonce() {
				continue
			}
			if other.GetStart() <= pos && pos < other.GetStart()+other.GetLength() {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func maxOverlapDepth(ranges []*file.Range) int {
	var maxDepth int
	for _, rng := range ranges {
		end := rng.GetStart() + rng.GetLength()
		for pos := rng.GetStart(); pos < end; pos++ {
			var depth int
			for _, other := range ranges {
				if other.GetStart() <= pos && pos < other.GetStart()+other.GetLength() {
					depth++
				}
			}
			if depth > maxDepth {
				maxDepth = depth
			}
		}
	}
	return maxDepth
}

func lookupScanLength(ranges []*file.Range, pos uint64) int {
	idxAfter := len(ranges)
	for i, rng := range ranges {
		if rng.GetStart() > pos {
			idxAfter = i
			break
		}
	}
	var scans int
	for i := idxAfter - 1; i >= 0; i-- {
		scans++
		rng := ranges[i]
		start := rng.GetStart()
		end := start + rng.GetLength()
		if start <= pos && pos < end {
			continue
		}
	}
	return scans
}
