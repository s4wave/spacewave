package transform_gzip

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

func TestDecodeBlockRejectsOversizedOutput(t *testing.T) {
	g, err := NewGzip(&Config{})
	if err != nil {
		t.Fatalf("NewGzip: %v", err)
	}

	encoded, err := g.EncodeBlock(bytes.Repeat([]byte("a"), block.MaxBlockSize+1))
	if err != nil {
		t.Fatalf("EncodeBlock: %v", err)
	}

	if _, err := g.DecodeBlock(encoded); err == nil {
		t.Fatal("expected oversized decoded block to fail")
	}
}

func TestEncodeBlockConcurrentReuse(t *testing.T) {
	for _, level := range []int{gzip.HuffmanOnly, gzip.DefaultCompression, gzip.BestSpeed, gzip.BestCompression} {
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			g, err := NewGzip(&Config{CompressionLevel: int32(level)})
			if err != nil {
				t.Fatal(err)
			}
			for worker := range 8 {
				t.Run(fmt.Sprint(worker), func(t *testing.T) {
					t.Parallel()
					for size := range 8 {
						data := bytes.Repeat([]byte{byte(worker), byte(size), 'a'}, size*100)
						var expected bytes.Buffer
						writer, err := gzip.NewWriterLevel(&expected, level)
						if err != nil {
							t.Fatal(err)
						}
						if _, err := writer.Write(data); err != nil {
							t.Fatal(err)
						}
						if err := writer.Close(); err != nil {
							t.Fatal(err)
						}
						encoded, err := g.EncodeBlock(data)
						if err != nil {
							t.Fatal(err)
						}
						if !bytes.Equal(encoded, expected.Bytes()) {
							t.Fatal("encoded block differs from a fresh gzip writer")
						}
						decoded, err := g.DecodeBlock(encoded)
						if err != nil {
							t.Fatal(err)
						}
						if !bytes.Equal(decoded, data) {
							t.Fatal("decoded block differs from input")
						}
					}
				})
			}
		})
	}
}

func BenchmarkEncodeBlock(b *testing.B) {
	g, err := NewGzip(&Config{CompressionLevel: gzip.BestSpeed})
	if err != nil {
		b.Fatal(err)
	}
	data := bytes.Repeat([]byte("block contents"), 1024)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := g.EncodeBlock(data); err != nil {
			b.Fatal(err)
		}
	}
}
