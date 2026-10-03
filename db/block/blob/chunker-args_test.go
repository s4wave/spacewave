package blob

import "testing"

// TestRabinArgsApplyArgs tests merging rabin chunker arguments.
func TestRabinArgsApplyArgs(t *testing.T) {
	// Merge a supplied polynomial and chunk boundaries into the Rabin arguments.
	c := &RabinArgs{}
	c.ApplyArgs(&RabinArgs{
		Pol:             0x3,
		ChunkingMinSize: 64,
		ChunkingMaxSize: 256,
	})

	// Verify the Rabin arguments retain every supplied option.
	if c.Pol != 0x3 {
		t.Errorf("expected pol 0x3, got: %x", c.Pol)
	}
	if c.ChunkingMinSize != 64 {
		t.Errorf("expected min size 64, got: %d", c.ChunkingMinSize)
	}
	if c.ChunkingMaxSize != 256 {
		t.Errorf("expected max size 256, got: %d", c.ChunkingMaxSize)
	}
}
