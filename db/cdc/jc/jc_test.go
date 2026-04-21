package jc

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func TestNewJCDefaults(t *testing.T) {
	c, err := NewJC()
	if err != nil {
		t.Fatalf("NewJC() error = %v", err)
	}
	// Default values per implementation
	if c.minSize != 2*1024 {
		t.Fatalf("minSize = %d, want %d", c.minSize, 2*1024)
	}
	if c.targetSize != 8*1024 {
		t.Fatalf("targetSize = %d, want %d", c.targetSize, 8*1024)
	}
	if c.maxSize != 64*1024 {
		t.Fatalf("maxSize = %d, want %d", c.maxSize, 64*1024)
	}
}

func TestNewWithOptionsValidation(t *testing.T) {
	// invalid targetSize
	if _, err := NewWithOptions(1024, 8192, 0, nil); err != ErrTargetSize {
		t.Fatalf("expected ErrTargetSize, got %v", err)
	}
	if _, err := NewWithOptions(1024, 8192, 32, nil); err != ErrTargetSize {
		t.Fatalf("expected ErrTargetSize for <64, got %v", err)
	}
	if _, err := NewWithOptions(1024, 8192, 2*1024*1024*1024, nil); err != ErrTargetSize {
		t.Fatalf("expected ErrTargetSize for >1GB, got %v", err)
	}

	// invalid minSize
	if _, err := NewWithOptions(32, 8192, 4096, nil); err != ErrMinSize {
		t.Fatalf("expected ErrMinSize for <64, got %v", err)
	}
	if _, err := NewWithOptions(16*1024, 8192, 4096, nil); err != ErrMinSize {
		t.Fatalf("expected ErrMinSize for min>=target, got %v", err)
	}
	if _, err := NewWithOptions(2*1024*1024*1024, 8192, 4096, nil); err != ErrMinSize {
		t.Fatalf("expected ErrMinSize for >1GB, got %v", err)
	}

	// invalid maxSize
	if _, err := NewWithOptions(1024, 32, 4096, nil); err != ErrMaxSize {
		t.Fatalf("expected ErrMaxSize for <64, got %v", err)
	}
	if _, err := NewWithOptions(1024, 4096, 4096, nil); err != ErrMaxSize {
		t.Fatalf("expected ErrMaxSize for max<=target, got %v", err)
	}
	if _, err := NewWithOptions(1024, 2*1024*1024*1024, 4096, nil); err != ErrMaxSize {
		t.Fatalf("expected ErrMaxSize for >1GB, got %v", err)
	}

	// valid
	if _, err := NewWithOptions(1024, 128*1024, 8192, nil); err != nil {
		t.Fatalf("unexpected error for valid options: %v", err)
	}
}

func TestAlgorithm_SmallDataReturnsN(t *testing.T) {
	minSize := uint64(128)
	maxSize := uint64(1024)
	targetSize := uint64(256)
	c, err := NewWithOptions(minSize, maxSize, targetSize, nil)
	if err != nil {
		t.Fatalf("NewWithOptions error: %v", err)
	}

	data := make([]byte, 200) // < targetSize
	for i := range data {
		data[i] = byte(i)
	}

	got := c.Algorithm(data, len(data))
	if got != len(data) {
		t.Fatalf("Algorithm returned %d, want %d (n<=targetSize should return n)", got, len(data))
	}
}

func TestAlgorithm_MaxClampAndBounds(t *testing.T) {
	c, err := NewJC()
	if err != nil {
		t.Fatalf("NewJC error: %v", err)
	}
	// Create a large buffer to ensure n >= maxSize path
	data := make([]byte, 2*c.maxSize)
	for i := range data {
		data[i] = byte(i)
	}

	got := c.Algorithm(data, len(data))
	if got > c.maxSize {
		t.Fatalf("Algorithm returned %d > maxSize %d", got, c.maxSize)
	}
	// When n>targetSize, if a boundary is found it must be >= minSize
	// Otherwise, if none found, it returns n (clamped to maxSize).
	if got < c.minSize {
		t.Fatalf("Algorithm returned %d < minSize %d", got, c.minSize)
	}
}

func chunkAll(c *JC, data []byte) []int {
	out := make([]int, 0, len(data)/max(1, c.targetSize))
	offset := 0
	for offset < len(data) {
		n := len(data) - offset
		cs := c.Algorithm(data[offset:], n)
		if cs <= 0 {
			panic("Algorithm returned non-positive chunk size")
		}
		out = append(out, cs)
		offset += cs
	}
	return out
}

func TestChunkingInvariantsAndDeterminism_DefaultKey(t *testing.T) {
	c, err := NewJC()
	if err != nil {
		t.Fatalf("NewJC error: %v", err)
	}

	// Generate reproducible data
	rng := rand.New(rand.NewSource(1)) //nolint:gosec
	total := 2 * 1024 * 1024           // 2 MiB
	data := make([]byte, total)
	for i := range data {
		data[i] = byte(rng.Intn(256)) //nolint:gosec
	}

	chunks1 := chunkAll(c, data)
	// invariant checks
	sum := 0
	for i, sz := range chunks1 {
		if sz <= 0 {
			t.Fatalf("chunk %d size <= 0", i)
		}
		if i < len(chunks1)-1 {
			if sz < c.minSize {
				t.Fatalf("chunk %d size %d < minSize %d", i, sz, c.minSize)
			}
			if sz > c.maxSize {
				t.Fatalf("chunk %d size %d > maxSize %d", i, sz, c.maxSize)
			}
		} else {
			// last chunk can be anything between 1 and maxSize
			if sz > c.maxSize {
				t.Fatalf("last chunk size %d > maxSize %d", sz, c.maxSize)
			}
		}
		sum += sz
	}
	if sum != len(data) {
		t.Fatalf("sum of chunks = %d, want %d", sum, len(data))
	}

	// Determinism: same input -> same chunking
	chunks2 := chunkAll(c, data)
	if !reflect.DeepEqual(chunks1, chunks2) {
		t.Fatalf("chunking not deterministic; got %v vs %v", chunks1[:min(5, len(chunks1))], chunks2[:min(5, len(chunks2))])
	}
}

func TestStreamingChunker_DefaultKey(t *testing.T) {
	// Generate reproducible data
	rng := rand.New(rand.NewSource(1)) //nolint:gosec
	total := 2 * 1024 * 1024           // 2 MiB
	data := make([]byte, total)
	for i := range data {
		data[i] = byte(rng.Intn(256)) //nolint:gosec
	}

	// Test streaming chunker
	chunker, err := NewChunker(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("NewChunker error: %v", err)
	}

	chunks, err := chunkAllStreaming(chunker)
	if err != nil {
		t.Fatalf("chunkAllStreaming error: %v", err)
	}

	// invariant checks
	sum := 0
	for i, sz := range chunks {
		if sz <= 0 {
			t.Fatalf("chunk %d size <= 0", i)
		}
		if i < len(chunks)-1 {
			if sz < chunker.jc.minSize {
				t.Fatalf("chunk %d size %d < minSize %d", i, sz, chunker.jc.minSize)
			}
			if sz > chunker.jc.maxSize {
				t.Fatalf("chunk %d size %d > maxSize %d", i, sz, chunker.jc.maxSize)
			}
		} else {
			// last chunk can be anything between 1 and maxSize
			if sz > chunker.jc.maxSize {
				t.Fatalf("last chunk size %d > maxSize %d", sz, chunker.jc.maxSize)
			}
		}
		sum += sz
	}
	if sum != len(data) {
		t.Fatalf("sum of chunks = %d, want %d", sum, len(data))
	}

	// Test determinism by comparing with non-streaming version
	jc, err := NewJC()
	if err != nil {
		t.Fatalf("NewJC error: %v", err)
	}
	expectedChunks := chunkAll(jc, data)
	if !reflect.DeepEqual(chunks, expectedChunks) {
		t.Fatalf("streaming chunker produced different results than non-streaming; got %v vs %v", chunks[:min(5, len(chunks))], expectedChunks[:min(5, len(expectedChunks))])
	}
}

func TestChunkingWithLargeDefaultValues(t *testing.T) {
	// Test with the default values used in the blob chunker
	minSize := uint64(2048 * 125)      // 256000 bytes
	targetSize := uint64(512000)       // 512000 bytes
	maxSize := uint64(4096 * (64 * 3)) // 786432 bytes

	c, err := NewWithOptions(minSize, maxSize, targetSize, nil)
	if err != nil {
		t.Fatalf("NewWithOptions error: %v", err)
	}

	// Generate reproducible data larger than maxSize to test chunking behavior
	rng := rand.New(rand.NewSource(42)) //nolint:gosec
	total := 2 * int(maxSize)           // 2x maxSize to ensure multiple chunks
	data := make([]byte, total)
	for i := range data {
		data[i] = byte(rng.Intn(256)) //nolint:gosec
	}

	// Test non-streaming chunker
	chunks := chunkAll(c, data)

	// Verify invariants
	sum := 0
	for i, sz := range chunks {
		if sz <= 0 {
			t.Fatalf("chunk %d size <= 0", i)
		}
		if i < len(chunks)-1 {
			if sz < c.minSize {
				t.Fatalf("chunk %d size %d < minSize %d", i, sz, c.minSize)
			}
			if sz > c.maxSize {
				t.Fatalf("chunk %d size %d > maxSize %d", i, sz, c.maxSize)
			}
		} else {
			// last chunk can be anything between 1 and maxSize
			if sz > c.maxSize {
				t.Fatalf("last chunk size %d > maxSize %d", sz, c.maxSize)
			}
		}
		sum += sz
	}
	if sum != len(data) {
		t.Fatalf("sum of chunks = %d, want %d", sum, len(data))
	}

	// Test streaming chunker produces same results
	chunker, err := NewChunkerWithOptions(bytes.NewReader(data), minSize, maxSize, targetSize, nil)
	if err != nil {
		t.Fatalf("NewChunkerWithOptions error: %v", err)
	}

	streamingChunks, err := chunkAllStreaming(chunker)
	if err != nil {
		t.Fatalf("chunkAllStreaming error: %v", err)
	}

	if !reflect.DeepEqual(chunks, streamingChunks) {
		t.Fatalf("streaming chunker produced different results than non-streaming; got %v vs %v", streamingChunks[:min(3, len(streamingChunks))], chunks[:min(3, len(chunks))])
	}

	// Verify we get reasonable chunk count (not too many tiny chunks)
	expectedChunks := (total / int(targetSize)) + 2 // rough estimate with some buffer
	if len(chunks) > expectedChunks {
		t.Fatalf("got %d chunks, expected roughly %d or fewer (may indicate chunking issues)", len(chunks), expectedChunks)
	}
}

func TestKeyAffectsGAndChunking(t *testing.T) {
	minSize := uint64(2048)
	maxSize := uint64(65536)
	targetSize := uint64(8192)

	cDefault, err := NewWithOptions(minSize, maxSize, targetSize, nil)
	if err != nil {
		t.Fatalf("NewWithOptions default key error: %v", err)
	}
	keyA := []byte("key-A")
	keyB := []byte("key-A") // same key, should produce same G
	keyC := []byte("key-C") // different key, likely different G

	cA, err := NewWithOptions(minSize, maxSize, targetSize, keyA)
	if err != nil {
		t.Fatalf("NewWithOptions keyA error: %v", err)
	}
	cB, err := NewWithOptions(minSize, maxSize, targetSize, keyB)
	if err != nil {
		t.Fatalf("NewWithOptions keyB error: %v", err)
	}
	cC, err := NewWithOptions(minSize, maxSize, targetSize, keyC)
	if err != nil {
		t.Fatalf("NewWithOptions keyC error: %v", err)
	}

	// Same key -> identical G
	if !reflect.DeepEqual(cA.G, cB.G) {
		t.Fatalf("same key produced different G")
	}
	// Different key -> G should differ in at least one entry
	if reflect.DeepEqual(cA.G, cC.G) {
		t.Fatalf("different keys produced identical G (unexpected)")
	}
	// Non-empty key vs default key almost certainly differs
	if reflect.DeepEqual(cDefault.G, cA.G) {
		t.Fatalf("default key and custom key produced identical G (unexpected)")
	}

	// Also verify chunking still respects invariants with different keys
	data := bytes.Repeat([]byte{0xAB}, int(3*maxSize+1234)) //nolint:gosec
	chunks := chunkAll(cC, data)
	total := 0
	for i, sz := range chunks {
		if sz <= 0 {
			t.Fatalf("chunk %d size <= 0", i)
		}
		if i < len(chunks)-1 {
			if sz < cC.minSize || sz > cC.maxSize {
				t.Fatalf("chunk %d size %d out of bounds [%d,%d]", i, sz, cC.minSize, cC.maxSize)
			}
		} else if sz > cC.maxSize {
			t.Fatalf("last chunk size %d > maxSize %d", sz, cC.maxSize)
		}
		total += sz
	}
	if total != len(data) {
		t.Fatalf("sum of chunks = %d, want %d", total, len(data))
	}
}

func TestGenerateSpacedMaskAndEmbedMask(t *testing.T) {
	// Simple, small case to validate mask layout
	mask := generateSpacedMask(4, 8) // expect 10101010b = 0xAA
	if mask != 0xAA {
		t.Fatalf("generateSpacedMask(4,8) = 0x%X, want 0xAA", mask)
	}
	// Clear least significant 1-bit
	embedded := embedMask(mask)
	if embedded != 0xA8 { // 10101000b
		t.Fatalf("embedMask(0xAA) = 0x%X, want 0xA8", embedded)
	}
	// Edge cases
	if generateSpacedMask(0, 8) != 0 {
		t.Fatalf("expected 0 when oneCount=0")
	}
	if generateSpacedMask(8, 8) != 0xFFFFFFFFFFFFFFFF {
		t.Fatalf("expected all-ones when oneCount>=totalBits")
	}
}

func BenchmarkChunking_DefaultKey(b *testing.B) {
	c, err := NewJC()
	if err != nil {
		b.Fatalf("NewJC error: %v", err)
	}
	size := 8 * 1024 * 1024 // 8 MiB
	data := make([]byte, size)
	rng := rand.New(rand.NewSource(42)) //nolint:gosec
	for i := range data {
		data[i] = byte(rng.Intn(256)) //nolint:gosec
	}
	// Reusable slice to avoid reallocation noise
	chunks := make([]int, 0, size/c.targetSize)

	b.ReportAllocs()

	for b.Loop() {
		_ = chunkAllReuse(c, data, &chunks)
	}
}

func TestChunkerReset(t *testing.T) {
	data := bytes.Repeat([]byte("test data for reset"), 1000)
	reader := bytes.NewReader(data)
	chunker, err := NewChunker(reader)
	if err != nil {
		t.Fatalf("NewChunker error: %v", err)
	}

	// Read a chunk to populate internal buffers
	_, err = chunker.Next(nil)
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}

	// Verify buffers are populated
	if chunker.buf == nil || chunker.bufLen == 0 {
		t.Fatalf("expected buffers to be populated before reset")
	}

	// Store reference to JC and reader for verification
	originalJC := chunker.jc
	originalReader := chunker.reader

	// Reset the chunker
	chunker.Reset()

	// Verify state is cleared but JC and reader are preserved
	for _, v := range chunker.buf {
		if v != 0 {
			t.Fatalf("buf should be zeroed after reset")
		}
	}
	if chunker.bufLen != 0 {
		t.Fatalf("bufLen should be 0 after reset, got %d", chunker.bufLen)
	}
	if chunker.bufPos != 0 {
		t.Fatalf("bufPos should be 0 after reset, got %d", chunker.bufPos)
	}
	if chunker.pos != 0 {
		t.Fatalf("pos should be 0 after reset, got %d", chunker.pos)
	}
	if chunker.eof != false {
		t.Fatalf("eof should be false after reset")
	}
	if chunker.jc != originalJC {
		t.Fatalf("jc should be preserved after reset")
	}
	if chunker.reader != originalReader {
		t.Fatalf("reader should be preserved after reset")
	}

	// Test that chunker can still be used after reset (though reader is already consumed)
	// Reset the reader to beginning for reuse test
	reader.Reset(data)
	chunker.buf = make([]byte, chunker.jc.maxSize+chunker.jc.minSize)

	// Should be able to read chunks again
	chunk, err := chunker.Next(nil)
	if err != nil {
		t.Fatalf("Next after reset error: %v", err)
	}
	if chunk.Length <= 0 {
		t.Fatalf("expected valid chunk after reset, got length %d", chunk.Length)
	}
}

func BenchmarkChunking_CustomKey_32MiB(b *testing.B) {
	minSize := uint64(2048)
	maxSize := uint64(128 * 1024)
	targetSize := uint64(16 * 1024)
	key := []byte("benchmark-key-" + time.Now().Format(time.RFC3339Nano)) // just to avoid constant folding

	c, err := NewWithOptions(minSize, maxSize, targetSize, key)
	if err != nil {
		b.Fatalf("NewWithOptions error: %v", err)
	}
	size := 32 * 1024 * 1024 // 32 MiB
	data := make([]byte, size)
	rng := rand.New(rand.NewSource(1337)) //nolint:gosec
	for i := range data {
		data[i] = byte(rng.Intn(256)) //nolint:gosec
	}
	chunks := make([]int, 0, size/c.targetSize)

	b.ReportAllocs()

	for b.Loop() {
		_ = chunkAllReuse(c, data, &chunks)
	}
}

func chunkAllStreaming(chunker *Chunker) ([]int, error) {
	var out []int
	var buf []byte
	for {
		chunk, err := chunker.Next(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if chunk.Length <= 0 {
			return nil, errors.New("chunker returned non-positive chunk size")
		}
		out = append(out, chunk.Length)

		// Grow buffer if needed for next iteration
		if len(buf) < chunk.Length*2 {
			buf = make([]byte, chunk.Length*2)
		}
	}
	return out, nil
}

// chunkAllReuse is like chunkAll but reuses the provided slice to reduce allocations in benchmarks.
func chunkAllReuse(c *JC, data []byte, out *[]int) []int {
	res := (*out)[:0]
	offset := 0
	for offset < len(data) {
		n := len(data) - offset
		cs := c.Algorithm(data[offset:], n)
		if cs <= 0 {
			panic("Algorithm returned non-positive chunk size")
		}
		res = append(res, cs)
		offset += cs
	}
	*out = res
	return res
}
