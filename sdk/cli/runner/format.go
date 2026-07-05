package runner

import (
	"bytes"
	"io"
	"text/tabwriter"

	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
)

func newMarshalBuf() (*bytes.Buffer, *protojson.MarshalState) {
	var buf bytes.Buffer
	ms := protojson.NewMarshalState(protojson.MarshalerConfig{}, protojson.NewJsonStream(&buf))
	return &buf, ms
}

func formatOutput(w io.Writer, jsonData []byte, format string) error {
	switch format {
	case "json":
		jsonData = append(jsonData, '\n')
		_, err := w.Write(jsonData)
		return err
	case "yaml":
		yamlData, err := yaml.JSONToYAML(jsonData)
		if err != nil {
			return errors.Wrap(err, "convert json to yaml")
		}
		_, err = w.Write(yamlData)
		return err
	case "text", "table", "":
		return nil
	default:
		return errors.Errorf("unsupported output format: %s", format)
	}
}

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

func writeTable(w io.Writer, indent string, rows [][]string) {
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

func truncateID(id string, max int) string {
	if len(id) <= max {
		return id
	}
	return id[:max] + "..."
}

func writeJSONStringField(ms *protojson.MarshalState, more *bool, name, value string) {
	ms.WriteMoreIf(more)
	ms.WriteObjectField(name)
	ms.WriteString(value)
}

func writeJSONUint64Field(ms *protojson.MarshalState, more *bool, name string, value uint64) {
	ms.WriteMoreIf(more)
	ms.WriteObjectField(name)
	ms.WriteUint64(value)
}

func writeJSONBoolField(ms *protojson.MarshalState, more *bool, name string, value bool) {
	ms.WriteMoreIf(more)
	ms.WriteObjectField(name)
	ms.WriteBool(value)
}
