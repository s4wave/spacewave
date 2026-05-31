package s4wave_layout

import (
	"bytes"
	"os"
	"testing"
)

func TestGeneratedLayoutSRPCExcludesGoScript(t *testing.T) {
	src, err := os.ReadFile("layout_srpc.pb.go")
	if err != nil {
		t.Fatalf("read generated layout SRPC file: %v", err)
	}
	if !bytes.HasPrefix(src, []byte("//go:build !goscript\n\n")) {
		t.Fatal("layout_srpc.pb.go must stay excluded from goscript builds; keep layout-srpc-goscript.go as the goscript owner")
	}
}
