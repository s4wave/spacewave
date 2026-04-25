package sobject

import (
	"testing"

	block_transform "github.com/s4wave/spacewave/db/block/transform"
	blockenc_conf "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
)

func TestRedactStepConfig(t *testing.T) {
	// Build a blockenc config with a key.
	enc := &blockenc_conf.Config{
		BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
		Key:      []byte("0123456789abcdef0123456789abcdef"),
	}
	data, err := enc.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	step := &block_transform.StepConfig{
		Id:     blockenc_conf.ConfigID,
		Config: data,
	}

	redacted := RedactStepConfig(step)
	if redacted.GetId() != blockenc_conf.ConfigID {
		t.Fatal("expected blockenc config ID")
	}

	// Decode the redacted config.
	out := &blockenc_conf.Config{}
	if err := out.UnmarshalVT(redacted.GetConfig()); err != nil {
		t.Fatal(err)
	}
	if out.GetBlockEnc() != blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305 {
		t.Fatalf("expected XCHACHA20_POLY1305, got %v", out.GetBlockEnc())
	}
	if len(out.GetKey()) != 0 {
		t.Fatalf("expected key to be zeroed, got %d bytes", len(out.GetKey()))
	}

	// Verify original is unchanged.
	orig := &blockenc_conf.Config{}
	if err := orig.UnmarshalVT(step.GetConfig()); err != nil {
		t.Fatal(err)
	}
	if len(orig.GetKey()) != 32 {
		t.Fatal("original key was modified")
	}
}

func TestRedactStepConfig_NonBlockenc(t *testing.T) {
	step := &block_transform.StepConfig{
		Id:     "hydra/transform/lz4",
		Config: []byte{0x01, 0x02, 0x03},
	}
	redacted := RedactStepConfig(step)
	if redacted.GetId() != "hydra/transform/lz4" {
		t.Fatal("unexpected id change")
	}
	if len(redacted.GetConfig()) != 3 {
		t.Fatal("config bytes changed for non-blockenc step")
	}
}
