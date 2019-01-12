package transform_all

import (
	"bytes"
	// "math/rand"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
)

// TestAllTransforms tests all transforms.
func TestAllTransforms(t *testing.T) {
	for _, sf := range BuildFactories() {
		p := make([]byte, 1500)
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
			sf.ConstructConfig(),
			controller.ConstructOpts{},
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		o, err := s.EncodeBlock(p)
		if err != nil {
			t.Fatal(err.Error())
		}
		ol := len(o)
		oi, err := s.DecodeBlock(o)
		if err != nil {
			t.Fatal(err.Error())
		}
		if bytes.Compare(f, oi) != 0 {
			t.Fail()
		}
		t.Logf(
			"pass: %s, %d bytes -> %d bytes",
			sf.GetConfigID(),
			len(p), ol,
		)
	}
}
