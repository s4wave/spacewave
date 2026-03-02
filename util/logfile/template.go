package logfile

import (
	"strconv"
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
	y := zeroPad(ts.Year(), 4)
	mo := zeroPad(int(ts.Month()), 2)
	d := zeroPad(ts.Day(), 2)
	h := zeroPad(ts.Hour(), 2)
	mi := zeroPad(ts.Minute(), 2)
	s := zeroPad(ts.Second(), 2)

	path = strings.ReplaceAll(path, "{ts}", y+mo+d+"-"+h+mi+s)
	path = strings.ReplaceAll(path, "{YYYY}", y)
	path = strings.ReplaceAll(path, "{MM}", mo)
	path = strings.ReplaceAll(path, "{DD}", d)
	path = strings.ReplaceAll(path, "{HH}", h)
	path = strings.ReplaceAll(path, "{mm}", mi)
	path = strings.ReplaceAll(path, "{ss}", s)

	return path
}

// zeroPad pads n with leading zeros to the given width.
func zeroPad(n int, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}
