package spacewave_cli

import (
	"math"
	"testing"
)

func TestSessionIndexFromIntRejectsOverflow(t *testing.T) {
	if _, err := sessionIndexFromInt(math.MaxInt); err == nil {
		t.Fatal("sessionIndexFromInt accepted an out-of-range session index")
	}
}

func TestVMMemoryMiBRejectsOverflow(t *testing.T) {
	if _, err := vmMemoryMiB(math.MaxUint32 + 1); err == nil {
		t.Fatal("vmMemoryMiB accepted an out-of-range configuration value")
	}
}
