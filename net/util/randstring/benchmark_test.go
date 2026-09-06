package randstring

import "testing"

// BenchmarkRandomIdentifier measures fresh eight-letter identifiers.
func BenchmarkRandomIdentifier(b *testing.B) {
	for b.Loop() {
		RandomIdentifier(8)
	}
}
