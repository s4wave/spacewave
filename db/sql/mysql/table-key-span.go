package mysql

import (
	"bytes"
	"encoding/binary"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/s4wave/spacewave/db/kvtx"
)

// keySpan selects the keys under a prefix whose next component lies between
// two inclusive bounds. Each bound is an encoded component: 0 for NULL, or 1
// and an ordered word (see orderedWord). The span may select more keys than its
// range, never fewer. The engine filters the rest because PreciseMatch is
// false.
type keySpan struct {
	// lower is the least component, or nil to start at the prefix.
	lower []byte
	// upper is the greatest component, or nil to walk to the prefix's end.
	upper []byte
}

// nullComponent bounds the NULL and non-NULL keys of a column together with
// valueComponent and afterValues.
var (
	nullComponent  = []byte{0}
	valueComponent = []byte{1}
	afterValues    = []byte{2}
)

// spanOf returns the keys of a column of typ that column selects. typ must be
// an orderedType.
func spanOf(ctx *sql.Context, typ sql.Type, column sql.MySQLRangeColumnExpr) keySpan {
	// A lower cut at NULL starts at the prefix, so the span includes NULL.
	var span keySpan
	switch cut := column.LowerBound.(type) {
	case sql.AboveNull:
		span.lower = valueComponent
	case sql.Below:
		span.lower = boundComponent(ctx, typ, cut.Key, true)
	case sql.Above:
		span.lower = boundComponent(ctx, typ, cut.Key, true)
	}

	// An upper cut at AboveAll walks to the prefix's end.
	switch cut := column.UpperBound.(type) {
	case sql.BelowNull, sql.AboveNull:
		span.upper = nullComponent
	case sql.Below:
		span.upper = boundComponent(ctx, typ, cut.Key, false)
	case sql.Above:
		span.upper = boundComponent(ctx, typ, cut.Key, false)
	}
	return span
}

// boundComponent encodes a range bound as an inclusive component bound that
// holds every key of a value of typ within the range. Converting a number
// rounds it to the nearest value of typ, which never passes the least value
// above a lower bound or the greatest value below an upper bound. A bound past
// either end of typ's range selects all or none of its values. GMS wraps a
// negative into an unsigned type, so the converted value is not used there.
// It returns nil, an open bound, when the key does not convert.
func boundComponent(ctx *sql.Context, typ sql.Type, key any, lower bool) []byte {
	// Convert the bound as stored values were converted.
	value, inRange, err := keyValue(ctx, typ, key)
	if err != nil {
		return nil
	}

	// Place a bound beyond typ's range at that end of the column's keys.
	switch inRange {
	case sql.Underflow:
		if lower {
			return valueComponent
		}
		return nullComponent
	case sql.Overflow:
		if lower {
			return afterValues
		}
		return nil
	}

	// Encode the converted value as a stored key component.
	word, ok := orderedWord(value)
	if !ok {
		return nil
	}
	return binary.BigEndian.AppendUint64(bytes.Clone(valueComponent), word)
}

// walk calls fn at each entry of it whose key is prefix followed by a component
// within the span, in key order. it must iterate prefix in sorted order.
func (s keySpan) walk(it kvtx.Iterator, prefix []byte, fn func() error) error {
	// Seek to the first key at or above the lower bound.
	start := append(bytes.Clone(prefix), s.lower...)
	if err := it.Seek(start); err != nil {
		return err
	}

	// Stop at the first component above the upper bound.
	for ; it.Valid(); it.Next() {
		if s.upper != nil {
			component := it.Key()[len(prefix):]
			if bytes.Compare(component[:min(len(component), len(s.upper))], s.upper) > 0 {
				break
			}
		}
		if err := fn(); err != nil {
			return err
		}
	}
	return it.Err()
}
