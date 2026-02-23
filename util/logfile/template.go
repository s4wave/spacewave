package logfile

import (
	"fmt"
	"strings"
	"time"
)

// ExpandTemplate expands timestamp templates in the given path string.
//
// Supported templates:
//   - {ts} -> YYYYMMDD-HHMMSS
//   - {YYYY} -> 4-digit year
//   - {MM} -> 2-digit month (zero-padded)
//   - {DD} -> 2-digit day (zero-padded)
//   - {HH} -> 2-digit hour (zero-padded, 24h)
//   - {mm} -> 2-digit minute (zero-padded)
//   - {ss} -> 2-digit second (zero-padded)
func ExpandTemplate(path string, ts time.Time) string {
	y := fmt.Sprintf("%04d", ts.Year())
	mo := fmt.Sprintf("%02d", ts.Month())
	d := fmt.Sprintf("%02d", ts.Day())
	h := fmt.Sprintf("%02d", ts.Hour())
	mi := fmt.Sprintf("%02d", ts.Minute())
	s := fmt.Sprintf("%02d", ts.Second())

	path = strings.ReplaceAll(path, "{ts}", y+mo+d+"-"+h+mi+s)
	path = strings.ReplaceAll(path, "{YYYY}", y)
	path = strings.ReplaceAll(path, "{MM}", mo)
	path = strings.ReplaceAll(path, "{DD}", d)
	path = strings.ReplaceAll(path, "{HH}", h)
	path = strings.ReplaceAll(path, "{mm}", mi)
	path = strings.ReplaceAll(path, "{ss}", s)

	return path
}
