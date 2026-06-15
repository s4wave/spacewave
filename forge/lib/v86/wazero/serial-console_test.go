package v86_wazero

import (
	"bytes"
	"testing"
)

func TestGuestHaltDetectingWriterSplitMarker(t *testing.T) {
	var out bytes.Buffer
	w := &guestHaltDetectingWriter{dst: &out}
	for _, chunk := range []string{
		"Requesting system poweroff\r\n",
		"reboot: Power off not available: System ",
		"halted instead\r\n",
	} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatalf("write chunk: %v", err)
		}
	}
	if !w.Halted() {
		t.Fatal("expected split System halted marker to stop the serial console")
	}
	if got := out.String(); !bytes.Contains([]byte(got), []byte("System halted")) {
		t.Fatalf("expected writer to forward serial output, got %q", got)
	}
}
