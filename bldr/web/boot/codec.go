package boot

import protojson "github.com/aperturerobotics/protobuf-go-lite/json"

// MarshalBootReportJSON encodes a BootReport as deterministic proto JSON.
func MarshalBootReportJSON(report *BootReport) ([]byte, error) {
	return (protojson.MarshalerConfig{EnumsAsInts: false}).Marshal(report)
}
