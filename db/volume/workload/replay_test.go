package workload

import (
	"math"
	"strconv"
	"testing"
)

// TestReplaySize checks both native integer boundaries without allocating payloads.
func TestReplaySize(t *testing.T) {
	// Valid native sizes round-trip without narrowing.
	for _, size := range []int64{0, 1, math.MaxInt} {
		got, err := replaySize(size)
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if int64(got) != size {
			t.Fatalf("size %d converted to %d", size, got)
		}
	}

	// A 32-bit replay must reject sizes recorded on a 64-bit host.
	invalid := []int64{math.MinInt64, -1}
	if strconv.IntSize == 32 {
		invalid = append(invalid, math.MaxInt32+1, math.MaxInt64)
	}
	for _, size := range invalid {
		if _, err := replaySize(size); err == nil {
			t.Fatalf("accepted out-of-range size %d", size)
		}
	}
}

// TestNewReplaySizes checks value, block, and iterator sizes from decoded records.
func TestNewReplaySizes(t *testing.T) {
	// Include missing reads, an existence-only value, and a synthetic iterator key.
	records, err := ParseRecords("get 1 0 5 61\nset 1 0 127 62\nget 1 0 -1 63\nexists 1 0 1 64\niterate 2 1 0 70\niter-end 2 0 1 70\nget-block 0 0 13 65\n")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := NewReplay(records)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.values) != 127 {
		t.Fatalf("value buffer size = %d", len(replay.values))
	}
	for key, size := range map[string]int{"a": 5, "d": defaultValueSize, "p\xffreplay-0": defaultValueSize} {
		if replay.seedValues[key] != size {
			t.Fatalf("seed value %q size = %d, want %d", key, replay.seedValues[key], size)
		}
	}
	if _, ok := replay.seedValues["c"]; ok {
		t.Fatal("seeded a missing value")
	}
	if len(replay.seedBlocks) != 1 {
		t.Fatalf("seed block count = %d, want 1", len(replay.seedBlocks))
	}
	if len(replay.seedBlocks[0].data) != 13 {
		t.Fatalf("seed block size = %d, want 13", len(replay.seedBlocks[0].data))
	}

	// Malformed lengths must fail preparation before a slice or allocation uses them.
	for _, op := range []Op{OpSet, OpGet, OpIterEnd} {
		if _, err := NewReplay([]Record{{Op: op, Size: -2}}); err == nil {
			t.Fatalf("accepted negative %s size", op)
		}
	}
}

// TestNewReplayRejectsWideSizes exercises every narrowing path on 32-bit hosts.
func TestNewReplayRejectsWideSizes(t *testing.T) {
	if strconv.IntSize != 32 {
		t.Skip("int64 sizes only exceed native int on 32-bit hosts")
	}
	for _, op := range []Op{OpSet, OpGet, OpPut, OpGetBlock, OpStatBlock, OpIterEnd} {
		if _, err := NewReplay([]Record{{Op: op, Size: math.MaxInt32 + 1}}); err == nil {
			t.Fatalf("accepted overflowing %s size", op)
		}
	}
}
