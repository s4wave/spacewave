//go:build !js

package bldr_project_starlark

import "io"

// headWriter keeps the first limit bytes written to it and discards the rest,
// so a noisy process cannot grow it. Writes never fail, so the process never
// blocks on a full pipe.
type headWriter struct {
	// buf holds the bytes kept so far.
	buf []byte
	// limit is the number of bytes to keep.
	limit int
}

// Write implements io.Writer.
func (w *headWriter) Write(p []byte) (int, error) {
	if room := w.limit - len(w.buf); room > 0 {
		w.buf = append(w.buf, p[:min(room, len(p))]...)
	}
	return len(p), nil
}

var _ io.Writer = (*headWriter)(nil)
