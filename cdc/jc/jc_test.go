package jc

import (
	"bytes"
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
	if c.normalSize != 8*1024 {
		t.Fatalf("normalSize = %d, want %d", c.normalSize, 8*1024)
	}
	if c.maxSize != 64*1024 {
		t.Fatalf("maxSize = %d, want %d", c.maxSize, 64*1024)
	}
}

func TestNewWithOptionsValidation(t *testing.T) {
	// invalid normalSize
	if _, err := NewWithOptions(1024, 8192, 0, nil); err != ErrNormalSize {
		t.Fatalf("expected ErrNormalSize, got %v", err)
	}
	if _, err := NewWithOptions(1024, 8192, 32, nil); err != ErrNormalSize {
		t.Fatalf("expected ErrNormalSize for <64, got %v", err)
	}
	if _, err := NewWithOptions(1024, 8192, 2*1024*1024*1024, nil); err != ErrNormalSize {
		t.Fatalf("expected ErrNormalSize for >1GB, got %v", err)
	}

	// invalid minSize
	if _, err := NewWithOptions(32, 8192, 4096, nil); err != ErrMinSize {
		t.Fatalf("expected ErrMinSize for <64, got %v", err)
	}
	if _, err := NewWithOptions(16*1024, 8192, 4096, nil); err != ErrMinSize {
		t.Fatalf("expected ErrMinSize for min>=normal, got %v", err)
	}
	if _, err := NewWithOptions(2*1024*1024*1024, 8192, 4096, nil); err != ErrMinSize {
		t.Fatalf("expected ErrMinSize for >1GB, got %v", err)
	}

	// invalid maxSize
	if _, err := NewWithOptions(1024, 32, 4096, nil); err != ErrMaxSize {
		t.Fatalf("expected ErrMaxSize for <64, got %v", err)
	}
	if _, err := NewWithOptions(1024, 4096, 4096, nil); err != ErrMaxSize {
		t.Fatalf("expected ErrMaxSize for max<=normal, got %v", err)
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
	minSize := 128
	maxSize := 1024
	normalSize := 256
	c, err := NewWithOptions(minSize, maxSize, normalSize, nil)
	if err != nil {
		t.Fatalf("NewWithOptions error: %v", err)
	}

	data := make([]byte, 200) // < normalSize
	for i := range data {
		data[i] = byte(i)
	}

	got := c.Algorithm(data, len(data))
	if got != len(data) {
		t.Fatalf("Algorithm returned %d, want %d (n<=normalSize should return n)", got, len(data))
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
	// When n>normalSize, if a boundary is found it must be >= minSize
	// Otherwise, if none found, it returns n (clamped to maxSize).
	if got < c.minSize {
		t.Fatalf("Algorithm returned %d < minSize %d", got, c.minSize)
	}
}

func chunkAll(c *JC, data []byte) []int {
	out := make([]int, 0, len(data)/max(1, c.normalSize))
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
	rng := rand.New(rand.NewSource(1))
	total := 2 * 1024 * 1024 // 2 MiB
	data := make([]byte, total)
	for i := range data {
		data[i] = byte(rng.Intn(256))
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

func TestKeyAffectsGAndChunking(t *testing.T) {
	minSize := 2048
	maxSize := 65536
	normalSize := 8192

	cDefault, err := NewWithOptions(minSize, maxSize, normalSize, nil)
	if err != nil {
		t.Fatalf("NewWithOptions default key error: %v", err)
	}
	keyA := []byte("key-A")
	keyB := []byte("key-A") // same key, should produce same G
	keyC := []byte("key-C") // different key, likely different G

	cA, err := NewWithOptions(minSize, maxSize, normalSize, keyA)
	if err != nil {
		t.Fatalf("NewWithOptions keyA error: %v", err)
	}
	cB, err := NewWithOptions(minSize, maxSize, normalSize, keyB)
	if err != nil {
		t.Fatalf("NewWithOptions keyB error: %v", err)
	}
	cC, err := NewWithOptions(minSize, maxSize, normalSize, keyC)
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
	data := bytes.Repeat([]byte{0xAB}, 3*maxSize+1234)
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
	rng := rand.New(rand.NewSource(42))
	for i := range data {
		data[i] = byte(rng.Intn(256))
	}
	// Reusable slice to avoid reallocation noise
	chunks := make([]int, 0, size/c.normalSize)

	b.ReportAllocs()

	for b.Loop() {
		_ = chunkAllReuse(c, data, &chunks)
	}
}

func BenchmarkChunking_CustomKey_32MiB(b *testing.B) {
	minSize := 2048
	maxSize := 128 * 1024
	normalSize := 16 * 1024
	key := []byte("benchmark-key-" + time.Now().Format(time.RFC3339Nano)) // just to avoid constant folding

	c, err := NewWithOptions(minSize, maxSize, normalSize, key)
	if err != nil {
		b.Fatalf("NewWithOptions error: %v", err)
	}
	size := 32 * 1024 * 1024 // 32 MiB
	data := make([]byte, size)
	rng := rand.New(rand.NewSource(1337))
	for i := range data {
		data[i] = byte(rng.Intn(256))
	}
	chunks := make([]int, 0, size/c.normalSize)

	b.ReportAllocs()

	for b.Loop() {
		_ = chunkAllReuse(c, data, &chunks)
	}
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
