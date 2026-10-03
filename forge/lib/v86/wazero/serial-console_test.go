package v86_wazero

import (
	"bytes"
	"testing"
)

func TestGuestHaltDetectingWriterSplitMarker(t *testing.T) {
	// Create a serial writer that detects guest halt markers across writes.
	var out bytes.Buffer
	w := &guestHaltDetectingWriter{dst: &out}

	// Feed a guest halt marker split across consecutive serial writes.
	for _, chunk := range []string{
		"Requesting system poweroff\r\n",
		"reboot: Power off not available: System ",
		"halted instead\r\n",
	} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatalf("write chunk: %v", err)
		}
	}

	// Verify the serial writer detects the halt and forwards the full output.
	if !w.Halted() {
		t.Fatal("expected split System halted marker to stop the serial console")
	}
	if got := out.String(); !bytes.Contains([]byte(got), []byte("System halted")) {
		t.Fatalf("expected writer to forward serial output, got %q", got)
	}
}
