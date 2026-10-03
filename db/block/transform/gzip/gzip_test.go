package transform_gzip

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

func TestDecodeBlockRejectsOversizedOutput(t *testing.T) {
	// Construct the gzip transformer for the block size limit check.
	g, err := NewGzip(&Config{})
	if err != nil {
		t.Fatalf("NewGzip: %v", err)
	}

	// Compress an input that exceeds the maximum decoded block size.
	encoded, err := g.EncodeBlock(bytes.Repeat([]byte("a"), block.MaxBlockSize+1))
	if err != nil {
		t.Fatalf("EncodeBlock: %v", err)
	}

	// Verify the decoder rejects oversized gzip output.
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
	// Prepare the gzip encoder and representative block for allocation measurement.
	g, err := NewGzip(&Config{CompressionLevel: gzip.BestSpeed})
	if err != nil {
		b.Fatal(err)
	}
	data := bytes.Repeat([]byte("block contents"), 1024)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	// Measure gzip encoding with pooled writer reuse.
	for b.Loop() {
		if _, err := g.EncodeBlock(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeBlock(b *testing.B) {
	// Prepare an encoded block for gzip decoder allocation measurement.
	g, err := NewGzip(&Config{})
	if err != nil {
		b.Fatal(err)
	}
	data := bytes.Repeat([]byte("block contents"), 32)
	encoded, err := g.EncodeBlock(data)
	if err != nil {
		b.Fatal(err)
	}

	// Configure benchmark accounting for the original block size.
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	// Measure gzip decoding while verifying the original block bytes.
	for b.Loop() {
		// Decode the benchmark block through a pooled gzip reader.
		decoded, err := g.DecodeBlock(encoded)
		if err != nil {
			b.Fatal(err)
		}

		// Verify reader reuse preserves the benchmark block contents.
		if !bytes.Equal(decoded, data) {
			b.Fatal("decoded block differs from input")
		}
	}
}

func TestDecodeBlockRecoversAfterInvalidInput(t *testing.T) {
	// Prepare a valid gzip block for decoding after malformed input.
	g, err := NewGzip(&Config{})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("valid block after a failed decode")
	encoded, err := g.EncodeBlock(data)
	if err != nil {
		t.Fatal(err)
	}

	// Exercise decoder reuse after each malformed gzip input.
	for _, invalid := range [][]byte{nil, []byte("not gzip"), encoded[:len(encoded)-1]} {
		// Verify the gzip decoder rejects the malformed block.
		if _, err := g.DecodeBlock(invalid); err == nil {
			t.Fatal("invalid compressed block was accepted")
		}

		// Decode a valid block after returning the failed reader to the pool.
		decoded, err := g.DecodeBlock(encoded)
		if err != nil {
			t.Fatal(err)
		}

		// Verify the failed decode did not affect the next block contents.
		if !bytes.Equal(decoded, data) {
			t.Fatal("failed decode affected the following block")
		}
	}
}
