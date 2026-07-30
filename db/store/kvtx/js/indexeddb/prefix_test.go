package store_kvtx_indexeddb

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
)

func TestPrefixRange(t *testing.T) {
	prefix := []byte("ab")
	upper, _ := kvtx.PrefixSuccessor(prefix)
	tests := []struct {
		name string
		key  []byte
		want bool
	}{
		{name: "next byte max", key: []byte{'a', 'b', 0xff}, want: true},
		{name: "next byte max with suffix", key: []byte{'a', 'b', 0xff, 0x00}, want: true},
		{name: "preceding byte max", key: []byte{'a', 'b', 0xfe, 0xff}, want: true},
		{name: "first key without prefix", key: []byte("ac"), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := bytes.Compare(test.key, prefix) >= 0 &&
				(upper == nil || bytes.Compare(test.key, upper) < 0)
			if got != test.want {
				t.Fatalf("key %v in range [%v, %v) = %t, want %t", test.key, prefix, upper, got, test.want)
			}
		})
	}
}

func TestBuildPrefixRange(t *testing.T) {
	prefix := []byte("ab")
	upper, _ := kvtx.PrefixSuccessor(prefix)
	maxPrefix := []byte{0xff, 0xff}

	full := prefixRange{lower: prefixBoundPrefix, upper: prefixBoundUpper, upperOpen: true}
	fullUnbounded := prefixRange{lower: prefixBoundPrefix, upper: prefixBoundNone}

	tests := []struct {
		name    string
		prefix  []byte
		upper   []byte
		key     []byte
		reverse bool
		want    prefixRange
	}{
		{name: "forward first run", prefix: prefix, upper: upper, want: full},
		{name: "reverse first run", prefix: prefix, upper: upper, reverse: true, want: full},
		{
			name: "forward resume inside", prefix: prefix, upper: upper,
			key:  []byte{'a', 'b', 0xff},
			want: prefixRange{lower: prefixBoundKey, upper: prefixBoundUpper, upperOpen: true},
		},
		{
			name: "reverse resume inside", prefix: prefix, upper: upper,
			key: []byte{'a', 'b', 0xff}, reverse: true,
			want: prefixRange{lower: prefixBoundPrefix, upper: prefixBoundKey},
		},
		// A forward seek short of the range clamps up to the prefix, while a
		// reverse seek short of the range has nothing below it to return.
		{name: "forward resume below", prefix: prefix, upper: upper, key: []byte("aa"), want: full},
		{
			name: "reverse resume below", prefix: prefix, upper: upper,
			key: []byte("aa"), reverse: true, want: prefixRange{done: true},
		},
		// Mirrored for a seek past the range.
		{
			name: "forward resume above", prefix: prefix, upper: upper,
			key: []byte("ac"), want: prefixRange{done: true},
		},
		{
			name: "reverse resume above", prefix: prefix, upper: upper,
			key: []byte("ac"), reverse: true, want: full,
		},
		// An all-0xFF prefix has no key above it, so nothing is ever past it.
		{name: "unbounded first run", prefix: maxPrefix, want: fullUnbounded},
		{
			name: "unbounded forward resume", prefix: maxPrefix, key: []byte{0xff, 0xff, 0xff},
			want: prefixRange{lower: prefixBoundKey, upper: prefixBoundNone},
		},
		{
			name: "unbounded reverse resume", prefix: maxPrefix, key: []byte{0xff, 0xff, 0xff},
			reverse: true, want: prefixRange{lower: prefixBoundPrefix, upper: prefixBoundKey},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := buildPrefixRange(test.prefix, test.upper, test.upper != nil, test.key, test.reverse)
			if got != test.want {
				t.Fatalf(
					"buildPrefixRange(%v, %v, %v, %t) = %+v, want %+v",
					test.prefix, test.upper, test.key, test.reverse, got, test.want,
				)
			}
		})
	}
}
