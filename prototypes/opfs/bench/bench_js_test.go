//go:build js

package bench

import (
	"bytes"
	"math/rand/v2"
	"strconv"
	"testing"

	opfs "github.com/s4wave/spacewave/prototypes/opfs/go-opfs"
)

var benchSizes = []int{4 << 10, 64 << 10}

func BenchmarkOPFSWrite(b *testing.B) {
	for _, size := range benchSizes {
		size := size
		b.Run(sizeLabel(size), func(b *testing.B) {
			root, cleanup := setupBenchDir(b, "bench-write")
			defer cleanup()

			fh, err := root.GetFileHandle("data.bin", true)
			if err != nil {
				b.Fatal(err)
			}
			ops, err := fh.OpenFileOps()
			if err != nil {
				b.Fatal(err)
			}
			defer ops.Close()

			data := bytes.Repeat([]byte{0x61}, size)

			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := ops.WriteAt(data, int64(i*size))
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkOPFSRead(b *testing.B) {
	for _, size := range benchSizes {
		size := size
		b.Run(sizeLabel(size), func(b *testing.B) {
			root, cleanup := setupBenchDir(b, "bench-read")
			defer cleanup()

			fh, err := root.GetFileHandle("data.bin", true)
			if err != nil {
				b.Fatal(err)
			}
			ops, err := fh.OpenFileOps()
			if err != nil {
				b.Fatal(err)
			}
			defer ops.Close()

			// Seed file with data.
			data := bytes.Repeat([]byte{0x61}, size*b.N)
			_, err = ops.Write(data)
			if err != nil {
				b.Fatal(err)
			}
			if err := ops.Flush(); err != nil {
				b.Fatal(err)
			}

			buf := make([]byte, size)

			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := ops.ReadAt(buf, int64(i*size))
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkOPFSRandomRead(b *testing.B) {
	for _, size := range benchSizes {
		size := size
		b.Run(sizeLabel(size), func(b *testing.B) {
			root, cleanup := setupBenchDir(b, "bench-rand")
			defer cleanup()

			fh, err := root.GetFileHandle("data.bin", true)
			if err != nil {
				b.Fatal(err)
			}
			ops, err := fh.OpenFileOps()
			if err != nil {
				b.Fatal(err)
			}
			defer ops.Close()

			// Seed file with b.N chunks.
			totalSize := size * b.N
			if totalSize == 0 {
				totalSize = size
			}
			data := bytes.Repeat([]byte{0x42}, totalSize)
			_, err = ops.Write(data)
			if err != nil {
				b.Fatal(err)
			}
			if err := ops.Flush(); err != nil {
				b.Fatal(err)
			}

			// Build random offsets.
			chunks := totalSize / size
			if chunks == 0 {
				chunks = 1
			}
			offsets := make([]int, b.N)
			for i := range offsets {
				offsets[i] = rand.IntN(chunks) * size
			}

			buf := make([]byte, size)

			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := ops.ReadAt(buf, int64(offsets[i]))
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func setupBenchDir(b *testing.B, name string) (*opfs.DirectoryHandle, func()) {
	b.Helper()
	root, err := opfs.GetRootDirectory()
	if err != nil {
		b.Fatal(err)
	}
	dir, err := root.GetDirectoryHandle(name, true)
	if err != nil {
		b.Fatal(err)
	}
	return dir, func() {
		_ = root.RemoveEntry(name, true)
	}
}

func sizeLabel(size int) string {
	if size >= 1<<20 {
		return strconv.Itoa(size/(1<<20)) + "MiB"
	}
	if size >= 1<<10 {
		return strconv.Itoa(size/(1<<10)) + "KiB"
	}
	return strconv.Itoa(size) + "B"
}
