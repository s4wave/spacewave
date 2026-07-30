package kvtx

import (
	"bytes"
	"testing"
)

func TestPrefixSuccessor(t *testing.T) {
	tests := []struct {
		name   string
		prefix []byte
		want   []byte
		ok     bool
	}{
		{name: "empty", prefix: []byte{}, ok: false},
		{name: "single byte", prefix: []byte{0x12}, want: []byte{0x13}, ok: true},
		{name: "simple", prefix: []byte("aa/"), want: []byte("aa0"), ok: true},
		{name: "carry", prefix: []byte{'a', 0xff}, want: []byte{'b'}, ok: true},
		{name: "trailing max run", prefix: []byte{'a', 0xff, 0xff}, want: []byte{'b'}, ok: true},
		{name: "all max", prefix: []byte{0xff, 0xff}, ok: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := PrefixSuccessor(test.prefix)
			if ok != test.ok {
				t.Fatalf("ok = %t, want %t", ok, test.ok)
			}
			if !bytes.Equal(got, test.want) {
				t.Fatalf("successor = %q, want %q", got, test.want)
			}
		})
	}
}
