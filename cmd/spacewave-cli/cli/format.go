//go:build !js

package spacewave_cli

import (
	"bytes"
	"io"
	"os"
	"text/tabwriter"

	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
)

// newMarshalBuf creates a buffer-backed MarshalState for building JSON output.
func newMarshalBuf() (*bytes.Buffer, *protojson.MarshalState) {
	var buf bytes.Buffer
	ms := protojson.NewMarshalState(protojson.MarshalerConfig{}, protojson.NewJsonStream(&buf))
	return &buf, ms
}

// formatOutput writes pre-marshaled JSON bytes to stdout in the requested format.
func formatOutput(jsonData []byte, format string) error {
	switch format {
	case "json":
		jsonData = append(jsonData, '\n')
		_, err := os.Stdout.Write(jsonData)
		return err
	case "yaml":
		yamlData, err := yaml.JSONToYAML(jsonData)
		if err != nil {
			return errors.Wrap(err, "convert json to yaml")
		}
		_, err = os.Stdout.Write(yamlData)
		return err
	case "text", "table":
		return nil
	default:
		return errors.Errorf("unsupported output format: %s", format)
	}
}

// writeStderr writes a message to stderr.
func writeStderr(msg string) {
	os.Stderr.WriteString(msg)
}

// writeFields writes aligned label: value pairs to w.
// Labels are right-padded so all values start at the same column.
func writeFields(w io.Writer, pairs [][2]string) {
	maxLen := 0
	for _, p := range pairs {
		if len(p[0]) > maxLen {
			maxLen = len(p[0])
		}
	}
	for _, p := range pairs {
		label := p[0] + ":"
		io.WriteString(w, label)
		pad := maxLen + 4 - len(p[0])
		for range pad {
			io.WriteString(w, " ")
		}
		io.WriteString(w, p[1]+"\n")
	}
}

// writeTable writes tabwriter-aligned rows to w.
// The first row is treated as ALL CAPS headers.
// indent is prepended to each row (use "" for no indent, "  " for section content).
func writeTable(w *os.File, indent string, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		if indent != "" {
			tw.Write([]byte(indent))
		}
		for j, cell := range row {
			if j > 0 {
				tw.Write([]byte("\t"))
			}
			tw.Write([]byte(cell))
		}
		tw.Write([]byte("\n"))
	}
	tw.Flush()
}

// truncateID truncates an ID string to max characters, appending "..." if truncated.
func truncateID(id string, max int) string {
	if len(id) <= max {
		return id
	}
	return id[:max] + "..."
}
