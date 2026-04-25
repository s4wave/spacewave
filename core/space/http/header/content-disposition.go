// Package header provides shared HTTP header helpers for space HTTP handlers.
package space_http_header

import (
	"mime"
	"net/http"
	"strings"
)

// SetAttachmentHeader sets a safe Content-Disposition attachment header.
func SetAttachmentHeader(w http.ResponseWriter, filename string) {
	if filename == "" {
		filename = "download"
	}
	if disp := mime.FormatMediaType("attachment", map[string]string{"filename": filename}); disp != "" {
		w.Header().Set("Content-Disposition", disp)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+fallbackFilename(filename)+`"`)
}

func fallbackFilename(filename string) string {
	var b strings.Builder
	for _, r := range filename {
		switch {
		case r < 0x20 || r > 0x7e:
			b.WriteByte('_')
		case r == '"' || r == '\\' || r == '/':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "download"
	}
	return b.String()
}
