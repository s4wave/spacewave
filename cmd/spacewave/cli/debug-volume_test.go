//go:build !js

package spacewave_cli

import (
	"encoding/binary"
	"testing"
)

// TestVolumeKeyClassesStayBounded checks that unique binary keys with no
// separator narrow the class width instead of making a class per key, and
// that separated keys keep their own class.
func TestVolumeKeyClassesStayBounded(t *testing.T) {
	// Add more unique binary keys than the bound, and some separated keys.
	c := newVolumeKeyClasses([]byte("/"), 1, 48)
	key := []byte{0x02, 'c', 0x01, 0, 0, 0, 0, 0, 0, 0, 0}
	n := volumeKeyClassesMax * 3
	for i := range n {
		binary.BigEndian.PutUint64(key[3:], uint64(i)*0x9e3779b97f4a7c15) //nolint:gosec // Test keys only.
		c.add(key, len(key), 1)
	}
	for range 10 {
		c.add([]byte("h/abc"), 5, 2)
	}

	// The classes fit the bound and every key is still counted.
	if len(c.byClass) > volumeKeyClassesMax {
		t.Fatalf("classes = %d, want at most %d", len(c.byClass), volumeKeyClassesMax)
	}
	if c.width >= len(key) {
		t.Fatalf("width = %d, want narrowed below %d", c.width, len(key))
	}
	var keys uint64
	for _, kc := range c.byClass {
		keys += kc.keys
	}
	if keys != uint64(n+10) { //nolint:gosec // Test counts only.
		t.Fatalf("keys = %d, want %d", keys, n+10)
	}
	if kc := c.byClass["h/"]; kc == nil || kc.keys != 10 || kc.valueBytes != 20 {
		t.Fatalf("h/ class = %+v", kc)
	}
}
