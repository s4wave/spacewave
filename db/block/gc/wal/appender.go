//go:build js

package block_gc_wal

import (
	"context"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// Appender adapts a WAL Writer to satisfy block_gc.WALAppender.
// It converts between block_gc.RefEdge and the proto RefEdge type.
type Appender struct {
	writer *Writer
}

// NewAppender creates a WALAppender backed by the given Writer.
func NewAppender(w *Writer) *Appender {
	return &Appender{writer: w}
}

// Append converts the block_gc.RefEdge slices to proto RefEdge
// messages and delegates to the underlying Writer.
func (a *Appender) Append(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	protoAdds := make([]*RefEdge, len(adds))
	for i, e := range adds {
		protoAdds[i] = &RefEdge{Subject: e.Subject, Object: e.Object}
	}
	protoRemoves := make([]*RefEdge, len(removes))
	for i, e := range removes {
		protoRemoves[i] = &RefEdge{Subject: e.Subject, Object: e.Object}
	}
	return a.writer.Append(ctx, protoAdds, protoRemoves)
}

// _ is a type assertion
var _ block_gc.WALAppender = (*Appender)(nil)
