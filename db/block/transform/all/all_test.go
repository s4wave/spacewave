package transform_all

import (
	"bytes"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
)

// TestAllTransforms tests all transforms.
func TestAllTransforms(t *testing.T) {
	// use non-random pattern that is compressible
	p := make([]byte, 4096)
	for i := range p {
		p[i] = byte(i % 256)
	}

	for fi, sf := range BuildStepFactories() {
		for tci, tc := range sf.ConstructMockConfig() {
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
			if !bytes.Equal(f, oi) {
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

// TestAllTransforms_JSON tests configuring all transforms with json.
//
// Checks that the yaml parsing for StepConfig works.
func TestAllTransforms_JSON(t *testing.T) {
	for _, sf := range BuildStepFactories() {
		sfs := block_transform.NewStepFactorySet()
		sfs.AddStepFactory(sf)
		for tci, tc := range sf.ConstructMockConfig() {
			jsonDat, err := tc.MarshalJSON()
			if err != nil {
				t.Fatal(err.Error())
			}
			stepConfJSON, err := (&block_transform.StepConfig{
				Id:     tc.GetConfigID(),
				Config: jsonDat,
			}).MarshalJSON()
			if err != nil {
				t.Fatal(err.Error())
			}
			stepConf := &block_transform.StepConfig{}
			err = stepConf.UnmarshalJSON(stepConfJSON)
			if err != nil {
				t.Fatal(err.Error())
			}
			tc, sf, err := sfs.UnmarshalStepConfig(stepConf)
			if err != nil {
				t.Fatal(err.Error())
			}
			_, err = sf.Construct(
				tc,
				controller.ConstructOpts{},
			)
			if err != nil {
				t.Fatalf("fail[%d]: %v", tci+1, err.Error())
			}
		}
	}
}
