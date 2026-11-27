//go:build !js

package spacewave_cli

import (
	"strconv"

	"github.com/s4wave/spacewave/db/volume/restore"
)

// volumeExportFields formats the source and export counts in a field table.
func volumeExportFields(path string, r restore.ExportResult) [][2]string {
	return [][2]string{
		{"Volume", path},
		{"Refs read", strconv.FormatUint(r.Refs, 10)},
		{"Blocks exported", strconv.FormatUint(r.Exported, 10)},
		{"Bytes exported", strconv.FormatUint(r.Bytes, 10)},
		{"Already in directory", strconv.FormatUint(r.Already, 10)},
		{"Source lacks", strconv.FormatUint(r.Missing, 10)},
		{"Hash mismatch", strconv.FormatUint(r.Mismatch, 10)},
	}
}
