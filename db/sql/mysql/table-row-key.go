package mysql

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"
)

// tableKeyFormat is the TableRoot key format encodeTableKey writes.
const tableKeyFormat = 1

// encodeTableKey encodes SQL equality keys, one self-delimiting component per
// column. Values are converted to their column type. A number is a fixed-width
// word in SQL order (see orderedWord), so the keys of a number column sort as
// SQL sorts its values. Any other value is length framed, text by its
// collation's weights. This framing preserves equality but not order. NULL
// sorts before every value.
// The second result reports whether any component is NULL. Row and schema must
// have equal lengths. columns is nil or supplies one prefix definition per
// column.
func encodeTableKey(ctx *sql.Context, schema sql.Schema, row sql.Row, columns []*TableIndexColumn) ([]byte, bool, error) {
	// Encode each component independently so NULL and composite keys cannot collide.
	key := make([]byte, 0, len(row)*8)
	hasNull := false
	for ord, col := range schema {
		// Represent NULL separately from every non-null value, including empty text.
		if row[ord] == nil {
			key = append(key, 0)
			hasNull = true
			continue
		}

		// Convert values before encoding so probes and stored rows share an identity.
		value, _, err := keyValue(ctx, col.Type, row[ord])
		if err != nil {
			return nil, false, err
		}

		// Encode a number as its ordered word.
		if word, ok := orderedWord(value); ok {
			key = append(key, 1)
			key = binary.BigEndian.AppendUint64(key, word)
			continue
		}

		// Use SQL wire values for other scalar types and collation weights for strings.
		wire, err := col.Type.SQL(ctx, nil, value)
		if err != nil {
			return nil, false, err
		}
		data := wire.Raw()
		if typ, ok := col.Type.(sql.StringType); ok && typ.IsStringType() {
			// Apply index prefix lengths in characters, or bytes for binary columns.
			text := string(data)
			if len(columns) != 0 && columns[ord].GetLength() > 0 {
				length := int(columns[ord].GetLength())
				if types.IsBinaryType(typ) {
					text = text[:min(length, len(text))]
				} else {
					runes := []rune(text)
					text = string(runes[:min(length, len(runes))])
				}
			}

			// CHAR comparison discards trailing spaces before applying the collation.
			if types.IsChar(typ) {
				text = strings.TrimRight(text, " ")
			}
			var weights bytes.Buffer
			if err := typ.Collation().WriteWeightString(&weights, text); err != nil {
				return nil, false, err
			}
			data = weights.Bytes()
		}

		// Frame the non-null component with its byte length.
		key = append(key, 1)
		key = binary.AppendUvarint(key, uint64(len(data)))
		key = append(key, data...)
	}
	return key, hasNull, nil
}

// keyValue converts v to the value of typ that keys encode, with signed zero
// normalized to zero, which SQL compares equal to it.
func keyValue(ctx *sql.Context, typ sql.Type, v any) (any, sql.ConvertInRange, error) {
	// Convert to the column type and unwrap stored wrappers.
	value, inRange, err := typ.Convert(ctx, v)
	if err != nil {
		return nil, inRange, err
	}
	value, err = sql.UnwrapAny(ctx, value)
	if err != nil {
		return nil, inRange, err
	}

	// Normalize signed zero.
	switch v := value.(type) {
	case float32:
		if v == 0 {
			value = float32(0)
		}
	case float64:
		if v == 0 {
			value = float64(0)
		}
	}
	return value, inRange, nil
}

// orderedWord maps a converted number to a word whose unsigned order is the
// number's SQL order: signed integers with the sign bit flipped, and floats with
// the sign bit flipped when positive and every bit flipped when negative. It
// reports false for any other value, such as a DECIMAL.
func orderedWord(value any) (uint64, bool) {
	switch v := value.(type) {
	case int8:
		return uint64(int64(v)) ^ 1<<63, true //nolint:gosec // the flip maps signed order onto unsigned order.
	case int16:
		return uint64(int64(v)) ^ 1<<63, true //nolint:gosec // the flip maps signed order onto unsigned order.
	case int32:
		return uint64(int64(v)) ^ 1<<63, true //nolint:gosec // the flip maps signed order onto unsigned order.
	case int64:
		return uint64(v) ^ 1<<63, true //nolint:gosec // the flip maps signed order onto unsigned order.
	case int:
		return uint64(int64(v)) ^ 1<<63, true //nolint:gosec // the flip maps signed order onto unsigned order.
	case uint8:
		return uint64(v), true
	case uint16:
		return uint64(v), true
	case uint32:
		return uint64(v), true
	case uint64:
		return v, true
	case uint:
		return uint64(v), true
	case float32:
		return floatWord(float64(v)), true
	case float64:
		return floatWord(v), true
	}
	return 0, false
}

// floatWord maps f to a word in f's order.
func floatWord(f float64) uint64 {
	bits := math.Float64bits(f)
	if bits>>63 != 0 {
		return ^bits
	}
	return bits | 1<<63
}

// orderedType reports whether the keys of a column of typ sort in SQL order,
// so a span of its values is a span of keys: integers and floats, whose
// values convert to the numbers orderedWord encodes.
func orderedType(typ sql.Type) bool {
	return types.IsInteger(typ) || types.IsFloat(typ)
}

// rowKey encodes the primary columns, or all columns for a keyless row prefix.
func (t *Table) rowKey(ctx *sql.Context, row sql.Row) ([]byte, error) {
	if len(t.schema.PkOrdinals) == 0 {
		key, _, err := encodeTableKey(ctx, t.schema.Schema, row, nil)
		return key, err
	}
	key, _, err := t.columnKey(ctx, row, t.schema.PkOrdinals, nil)
	return key, err
}

// columnKey encodes the selected row columns in index order.
func (t *Table) columnKey(ctx *sql.Context, row sql.Row, ordinals []int, columns []*TableIndexColumn) ([]byte, bool, error) {
	// Project the row onto the index columns.
	values := make(sql.Row, len(ordinals))
	for i, ord := range ordinals {
		values[i] = row[ord]
	}
	return t.encodeColumns(ctx, ordinals, values, columns)
}

// encodeColumns encodes values for the table columns at ordinals, in order.
func (t *Table) encodeColumns(ctx *sql.Context, ordinals []int, values sql.Row, columns []*TableIndexColumn) ([]byte, bool, error) {
	schema := make(sql.Schema, len(ordinals))
	for i, ord := range ordinals {
		schema[i] = t.schema.Schema[ord]
	}
	return encodeTableKey(ctx, schema, values, columns)
}
