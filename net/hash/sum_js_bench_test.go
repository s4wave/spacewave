//go:build js

package hash

import (
	"bytes"
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
		2 * 1024,
		4 * 1024,
		8 * 1024,
		10 * 1024,
		12 * 1024,
		14 * 1024,
		16 * 1024,
		24 * 1024,
		32 * 1024,
		48 * 1024,
		64 * 1024,
		96 * 1024,
		128 * 1024,
		192 * 1024,
		256 * 1024,
		512 * 1024,
		1024 * 1024,
	}

	for _, size := range sizes {
		data := makeBenchmarkData(size)
		name := strconv.Itoa(size)

		b.Run(name+"/subtlecrypto", func(b *testing.B) {
			got, err := subtleCryptoDigest("SHA-256", data)
			if err != nil {
				b.Fatal(err)
			}
			want := sha256.Sum256(data)
			if !bytes.Equal(got, want[:]) {
				b.Fatal("SubtleCrypto SHA256 mismatch")
			}

			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchHashBytes, err = subtleCryptoDigest("SHA-256", data)
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

		b.Run(name+"/routed", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var err error
				benchHashBytes, err = HashType_HashType_SHA256.Sum(data)
				if err != nil {
					b.Fatal(err)
				}
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
