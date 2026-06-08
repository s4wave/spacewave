package volume

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

func TestResolveHashType(t *testing.T) {
	if got := ResolveHashType(0); got != block.DefaultHashType {
		t.Fatalf("expected zero hash type to resolve SHA256, got %s", got)
	}
	info := &VolumeInfo{HashType: hash.HashType_HashType_SHA256}
	if got := info.ResolveHashType(); got != hash.HashType_HashType_SHA256 {
		t.Fatalf("expected explicit SHA256 volume info to win, got %s", got)
	}
}
