package v86_wazero

import (
	"encoding/binary"
	"testing"
)

// TestHost9PFSHandleRejectsShortDeclaredSize checks that a frame declaring a
// size below the 9P header gets an EIO reply instead of a panic.
func TestHost9PFSHandleRejectsShortDeclaredSize(t *testing.T) {
	req := []byte{3, 0, 0, 0, 100, 9, 0}
	reply := (&Host9PFS{}).Handle(req)
	if len(reply) < 7 {
		t.Fatalf("reply too short: %v", reply)
	}
	if tag := binary.LittleEndian.Uint16(reply[5:]); tag != 9 {
		t.Fatalf("reply tag %d, want 9", tag)
	}
	if got, want := reply[4], p9Error(9, p9EIO)[4]; got != want {
		t.Fatalf("reply type %d, want error type %d", got, want)
	}
}
