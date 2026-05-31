//go:build !js

package termui

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"
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

func TestRunWithKeysForwardsInput(t *testing.T) {
	input, inputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer inputWriter.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	keys := make(chan byte, 1)
	done := make(chan error, 1)
	go func() {
		done <- RunWithKeys(
			ctx,
			input,
			&bytes.Buffer{},
			"initial",
			make(chan string),
			func(value string) string { return value },
			func(_ string, key byte) {
				keys <- key
				cancel()
			},
		)
	}()

	if _, err := inputWriter.Write([]byte{'o'}); err != nil {
		t.Fatal(err)
	}
	select {
	case key := <-keys:
		if key != 'o' {
			t.Fatalf("unexpected key %q", key)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for key")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for RunWithKeys")
	}
}
