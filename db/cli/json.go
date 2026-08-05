//go:build !js && !wasip1

package cli

import (
	"bytes"
	"io"

	json "github.com/goccy/go-json"
)

func writeIndentedJSON(out io.Writer, dat []byte) error {
	// Indent the JSON response into a reusable buffer.
	var buf bytes.Buffer
	if err := json.Indent(&buf, dat, "", "\t"); err != nil {
		return err
	}

	// Write the indented response and its trailing newline.
	if _, err := out.Write(buf.Bytes()); err != nil {
		return err
	}
	if _, err := out.Write([]byte{'\n'}); err != nil {
		return err
	}
	return nil
}
