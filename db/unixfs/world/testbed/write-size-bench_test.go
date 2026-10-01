package unixfs_world_testbed

import (
	"context"
	"crypto/rand"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/sirupsen/logrus"
)

// BenchmarkWriteSizeAmplification writes a 1 MiB file into a World-backed
// filesystem in pieces of several sizes and reports the block bytes submitted
// for storage per content byte. The count includes each FsWriteAtOp payload
// blob and every block later deduplicated by hash.
func BenchmarkWriteSizeAmplification(b *testing.B) {
	for _, size := range []int{4 << 10, 32 << 10, 1 << 20} {
		b.Run(strconv.Itoa(size>>10)+"KiB", func(b *testing.B) {
			benchmarkWriteSize(b, size)
		})
	}
}

// benchmarkWriteSize writes one new 1 MiB file per iteration in size pieces.
func benchmarkWriteSize(b *testing.B, size int) {
	// Build a World holding a writable filesystem.
	ctx := b.Context()
	tb, err := hydra_testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		b.Fatal(err)
	}
	root, _, err := BuildTestbed(tb, objKey, true)
	if err != nil {
		b.Fatal(err)
	}
	defer root.Release()

	// Write new random content each iteration so no block is shared.
	const fileSize = 1 << 20
	data := make([]byte, fileSize)

	// Count the blocks each file submits.
	var count, written uint64
	b.SetBytes(fileSize)
	b.ResetTimer()
	for i := range b.N {
		_, _ = rand.Read(data)
		wctx, counter := block.WithWriteCounter(ctx)
		writeBenchFile(b, wctx, root, "f"+strconv.Itoa(i), data, size)
		snap := counter.Snapshot()
		count += snap.BlockWriteCount
		written += snap.BlockWriteBytes
	}

	// Report the block bytes per content byte.
	b.ReportMetric(float64(count)/float64(b.N), "block-writes/op")
	b.ReportMetric(float64(written)/float64(b.N*fileSize), "block-bytes/content-byte")
}

// writeBenchFile creates name and writes data to it in size pieces.
func writeBenchFile(b *testing.B, ctx context.Context, root *unixfs.FSHandle, name string, data []byte, size int) {
	// Create the file.
	ts := time.Now()
	if err := root.Mknod(ctx, true, []string{name}, unixfs.NewFSCursorNodeType_File(), 0o644, ts); err != nil {
		b.Fatal(err)
	}
	h, _, err := root.LookupPath(ctx, name)
	if err != nil {
		b.Fatal(err)
	}
	defer h.Release()

	// Write the content in order.
	for off := 0; off < len(data); off += size {
		if err := h.WriteAt(ctx, int64(off), data[off:min(off+size, len(data))], ts); err != nil {
			b.Fatal(err)
		}
	}
}
