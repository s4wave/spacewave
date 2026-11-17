//go:build amd64 && gc && !purego

package sha256

import (
	"crypto/sha256"
	"math/rand/v2"
	"strconv"
	"testing"

	"golang.org/x/sys/cpu"
)

// TestAVXMatchesStdlib checks the AVX code against crypto/sha256 on every
// length to 4 KB and on random splits into writes, whichever code New picks.
func TestAVXMatchesStdlib(t *testing.T) {
	if !cpu.X86.HasAVX || !cpu.X86.HasSSSE3 {
		t.Skip("no AVX on this processor")
	}
	rng := rand.New(rand.NewPCG(1, 2))
	data := make([]byte, 4096+130)
	for i := range data {
		data[i] = byte(rng.Uint32())
	}
	for n := range len(data) + 1 {
		want := sha256.Sum256(data[:n])

		var d digest
		d.Reset()
		_, _ = d.Write(data[:n])
		if got := d.checkSum(); got != want {
			t.Fatalf("one write of %d bytes: got %x, want %x", n, got, want)
		}

		d.Reset()
		for rest := data[:n]; len(rest) > 0; {
			cut := min(len(rest), rng.IntN(150))
			_, _ = d.Write(rest[:cut])
			rest = rest[cut:]
		}
		first := d.Sum(nil)
		if second := d.Sum(nil); string(first) != string(want[:]) || string(second) != string(want[:]) {
			t.Fatalf("split writes of %d bytes: got %x then %x, want %x", n, first, second, want)
		}
	}
}

// TestPicksStdlibWithAVX2 checks the choice of code.
func TestPicksStdlibWithAVX2(t *testing.T) {
	want := cpu.X86.HasAVX && cpu.X86.HasSSSE3 && !(cpu.X86.HasAVX2 && cpu.X86.HasBMI2)
	if useAVX != want {
		t.Fatalf("useAVX %v, want %v", useAVX, want)
	}
	_, isDigest := New().(*digest)
	if isDigest != useAVX {
		t.Fatalf("New returned the AVX digest %v with useAVX %v", isDigest, useAVX)
	}
}

func BenchmarkSum256(b *testing.B) {
	for _, size := range []int{64, 256, 1024, 4096} {
		data := make([]byte, size)
		b.Run("stdlib/"+strconv.Itoa(size), func(b *testing.B) {
			b.SetBytes(int64(size))
			for b.Loop() {
				sha256.Sum256(data)
			}
		})
		if !cpu.X86.HasAVX {
			continue
		}
		b.Run("avx/"+strconv.Itoa(size), func(b *testing.B) {
			b.SetBytes(int64(size))
			for b.Loop() {
				var d digest
				d.Reset()
				_, _ = d.Write(data)
				d.checkSum()
			}
		})
	}
}
