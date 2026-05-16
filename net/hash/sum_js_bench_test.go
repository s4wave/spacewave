//go:build js

package hash

import (
	"crypto/sha256"
	"strconv"
	"testing"
)

var (
	benchHashArray [sha256.Size]byte
	benchHashBytes []byte
)

func BenchmarkSHA256Browser(b *testing.B) {
	sizes := []int{
		1024,
		64 * 1024,
		1024 * 1024,
	}

	for _, size := range sizes {
		data := makeBenchmarkData(size)
		name := strconv.Itoa(size)

		b.Run(name+"/subtlecrypto", func(b *testing.B) {
			got, err := HashType_HashType_SHA256.Sum(data)
			if err != nil {
				b.Fatal(err)
			}
			want := sha256.Sum256(data)
			if string(got) != string(want[:]) {
				b.Fatal("SubtleCrypto SHA256 mismatch")
			}

			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchHashBytes, err = HashType_HashType_SHA256.Sum(data)
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(name+"/wasm", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchHashArray = sha256.Sum256(data)
			}
		})
	}
}

func makeBenchmarkData(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte((i * 31) ^ (i >> 3))
	}
	return data
}
