//go:build !js

package termui

import (
	"bytes"
	"testing"
)

func TestWriteNormalizesNewlines(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, "one\ntwo\r\nthree\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "\x1b[H\x1b[2Jone\r\ntwo\r\nthree\r\n"
	if got := buf.String(); got != want {
		t.Fatalf("unexpected output:\nwant %q\n got %q", want, got)
	}
}
