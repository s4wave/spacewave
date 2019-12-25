package transform_all

import (
	"bytes"
	// "math/rand"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
)

// TestAllTransforms tests all transforms.
func TestAllTransforms(t *testing.T) {
	for fi, sf := range BuildFactories() {
		for tci, tc := range sf.ConstructMockConfig() {
			p := make([]byte, 128)
			for i := range p {
				p[i] = byte(i) % 255
			}
			/*
				_, err := rand.Read(p)
				if err != nil {
					t.Fatal(err.Error())
				}
			*/
			f := make([]byte, len(p))
			copy(f, p)
			s, err := sf.Construct(
				tc,
				controller.ConstructOpts{},
			)
			if err != nil {
				t.Fatalf("fail[%d]: %v", tci+1, err.Error())
			}
			o, err := s.EncodeBlock(p)
			if err != nil {
				t.Fatalf("fail[%d]: %v", tci+1, err.Error())
			}
			ol := len(o)
			oi, err := s.DecodeBlock(o)
			if err != nil {
				t.Fatalf("fail[%d]: %v", tci+1, err.Error())
			}
			if len(f) != len(oi) {
				t.Fatalf("decode lengths did not match: %v != expected %v", len(oi), len(f))
			}
			if bytes.Compare(f, oi) != 0 {
				t.Fatalf("decode did not match: %v != expected %v", oi, f)
			}
			t.Logf(
				"pass[%d][%d]: %s, %d bytes -> %d bytes",
				fi,
				tci,
				sf.GetConfigID(),
				len(p), ol,
			)
		}
	}
}
