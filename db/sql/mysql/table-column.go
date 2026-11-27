package mysql

import (
	"bytes"
	"context"
	"time"

	"github.com/cockroachdb/apd/v3"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// BuildTableColumn constructs a TableColumn by marshaling a col.
//
// bcs must be set.
func BuildTableColumn(
	ctx context.Context,
	bcs *block.Cursor,
	opts *blob.BuildBlobOpts,
	col any,
) (*TableColumn, error) {
	ntc := &TableColumn{}
	bcs.SetBlock(ntc, true)
	if err := ntc.setValue(ctx, bcs, opts, col); err != nil {
		return nil, err
	}
	return ntc, nil
}

// setValue replaces the column value with col. bcs must be located at t: a
// value too large to store inline keeps its chunks in bcs's block graph.
func (t *TableColumn) setValue(
	ctx context.Context,
	bcs *block.Cursor,
	opts *blob.BuildBlobOpts,
	col any,
) error {
	// Clear the previous value and its references.
	t.Value = nil
	bcs.ClearAllRefs()
	bcs.MarkDirty()

	// Encode the SQL column value in its stored scalar or blob representation.
	switch v := col.(type) {
	case nil:
	case bool:
		t.Value = &TableColumn_BoolValue{BoolValue: v}
	case int:
		t.Value = &TableColumn_IntValue{IntValue: int64(v)}
	case int8:
		t.Value = &TableColumn_IntValue{IntValue: int64(v)}
	case int16:
		t.Value = &TableColumn_IntValue{IntValue: int64(v)}
	case int32:
		t.Value = &TableColumn_IntValue{IntValue: int64(v)}
	case int64:
		t.Value = &TableColumn_IntValue{IntValue: v}
	case uint:
		t.Value = &TableColumn_UintValue{UintValue: uint64(v)}
	case uint8:
		t.Value = &TableColumn_UintValue{UintValue: uint64(v)}
	case uint16:
		t.Value = &TableColumn_UintValue{UintValue: uint64(v)}
	case uint32:
		t.Value = &TableColumn_UintValue{UintValue: uint64(v)}
	case uint64:
		t.Value = &TableColumn_UintValue{UintValue: v}
	case float32:
		t.Value = &TableColumn_FloatValue{FloatValue: float64(v)}
	case float64:
		t.Value = &TableColumn_FloatValue{FloatValue: v}
	case string:
		if err := t.setBlob(ctx, bcs, opts, 6, []byte(v), tableColumnBlobString); err != nil {
			return err
		}
	case []byte:
		if err := t.setBlob(ctx, bcs, opts, 7, v, tableColumnBlobBytes); err != nil {
			return err
		}
	case time.Time:
		nanos := v.Nanosecond()
		t.Value = &TableColumn_TimestampValue{
			TimestampValue: &TableTimestamp{
				UnixSeconds: v.Unix(),
				Nanos:       int32(nanos), // #nosec G115 -- Nanosecond is in [0, 999999999].
			},
		}
	case types.Timespan:
		t.Value = &TableColumn_TimespanMicros{TimespanMicros: int64(v)}
	case *apd.Decimal:
		t.Value = &TableColumn_DecimalText{DecimalText: v.String()}
	case sql.JSONWrapper:
		jsonString, err := encodeJsonWrapper(ctx, v)
		if err != nil {
			return err
		}
		if err := t.setBlob(ctx, bcs, opts, 8, []byte(jsonString), tableColumnBlobJson); err != nil {
			return err
		}
	default:
		return errors.Errorf("unsupported table column type %T", col)
	}

	return nil
}

// IsNil returns if the object is nil.
func (t *TableColumn) IsNil() bool {
	return t == nil
}

// IsEmpty checks if the table column is empty.
func (t *TableColumn) IsEmpty() bool {
	return t.GetValue() == nil
}

// blobValue returns the column's blob and its sub-block id, or nil for a
// scalar column.
func (t *TableColumn) blobValue() (*blob.Blob, uint32) {
	switch v := t.GetValue().(type) {
	case *TableColumn_StringBlob:
		return v.StringBlob, 6
	case *TableColumn_BytesBlob:
		return v.BytesBlob, 7
	case *TableColumn_JsonBlob:
		return v.JsonBlob, 8
	default:
		return nil, 0
	}
}

// readsThroughCursor reports whether the column's value lives in blob chunks
// outside the row block, so reading it needs a cursor at the column.
func (t *TableColumn) readsThroughCursor() bool {
	b, _ := t.blobValue()
	if b == nil {
		return false
	}
	_, inline := b.InlineData()
	return !inline
}

// fetchBlobData returns the bytes of the column's blob. An inline blob reads
// from the decoded row without a cursor. A chunked blob reads through bcs,
// located at the column.
func (t *TableColumn) fetchBlobData(ctx context.Context, bcs *block.Cursor) ([]byte, error) {
	b, id := t.blobValue()
	if data, ok := b.InlineData(); ok {
		return data, nil
	}
	return blob.FetchToBytes(ctx, bcs.FollowSubBlock(id))
}

// FetchSqlColumn converts the column into its SQL value. A scalar value or an
// inline blob decodes from the column itself. A chunked blob reads through bcs,
// located at the column, and reads as nil without one.
func (t *TableColumn) FetchSqlColumn(ctx context.Context, bcs *block.Cursor) (any, error) {
	if bcs == nil && t.readsThroughCursor() {
		return nil, nil
	}

	switch v := t.GetValue().(type) {
	case nil:
		return nil, nil
	case *TableColumn_BoolValue:
		return v.BoolValue, nil
	case *TableColumn_IntValue:
		return v.IntValue, nil
	case *TableColumn_UintValue:
		return v.UintValue, nil
	case *TableColumn_FloatValue:
		return v.FloatValue, nil
	case *TableColumn_StringBlob:
		data, err := t.fetchBlobData(ctx, bcs)
		if err != nil {
			return nil, err
		}
		return string(data), nil
	case *TableColumn_BytesBlob:
		// Copy so the caller cannot alias the decoded row block.
		data, err := t.fetchBlobData(ctx, bcs)
		if err != nil {
			return nil, err
		}
		return bytes.Clone(data), nil
	case *TableColumn_JsonBlob:
		data, err := t.fetchBlobData(ctx, bcs)
		if err != nil {
			return nil, err
		}
		out, _, err := types.JSON.Convert(ctx, string(data))
		return out, err
	case *TableColumn_TimestampValue:
		tv := v.TimestampValue
		return time.Unix(tv.GetUnixSeconds(), int64(tv.GetNanos())).UTC(), nil
	case *TableColumn_TimespanMicros:
		return types.Timespan(v.TimespanMicros), nil
	case *TableColumn_DecimalText:
		d, _, err := apd.NewFromString(v.DecimalText)
		return d, err
	default:
		return nil, errors.Errorf("unsupported table column value %T", v)
	}
}

// Validate cursorily validates the table column.
func (t *TableColumn) Validate() error {
	switch t.GetValue().(type) {
	case nil:
	case *TableColumn_BoolValue:
	case *TableColumn_IntValue:
	case *TableColumn_UintValue:
	case *TableColumn_FloatValue:
	case *TableColumn_TimestampValue:
	case *TableColumn_TimespanMicros:
	case *TableColumn_DecimalText:
	case *TableColumn_StringBlob:
		return t.GetStringBlob().Validate()
	case *TableColumn_BytesBlob:
		return t.GetBytesBlob().Validate()
	case *TableColumn_JsonBlob:
		return t.GetJsonBlob().Validate()
	default:
		return errors.Errorf("unsupported table column value %T", t.GetValue())
	}
	return nil
}

// ApplySubBlock applies a sub-block change with a field id.
func (t *TableColumn) ApplySubBlock(id uint32, next block.SubBlock) error {
	v, ok := next.(*blob.Blob)
	if !ok {
		return block.ErrUnexpectedType
	}
	switch id {
	case 6:
		t.Value = &TableColumn_StringBlob{StringBlob: v}
	case 7:
		t.Value = &TableColumn_BytesBlob{BytesBlob: v}
	case 8:
		t.Value = &TableColumn_JsonBlob{JsonBlob: v}
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (t *TableColumn) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	switch t.GetValue().(type) {
	case *TableColumn_StringBlob:
		m[6] = t.GetStringBlob()
	case *TableColumn_BytesBlob:
		m[7] = t.GetBytesBlob()
	case *TableColumn_JsonBlob:
		m[8] = t.GetJsonBlob()
	}
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (t *TableColumn) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 6:
		return t.blobCtor(tableColumnBlobString)
	case 7:
		return t.blobCtor(tableColumnBlobBytes)
	case 8:
		return t.blobCtor(tableColumnBlobJson)
	}
	return nil
}

func (t *TableColumn) setBlob(
	ctx context.Context,
	bcs *block.Cursor,
	opts *blob.BuildBlobOpts,
	id uint32,
	data []byte,
	kind tableColumnBlobKind,
) error {
	blk, err := blob.BuildBlob(ctx, int64(len(data)), bytes.NewReader(data), bcs.FollowSubBlock(id), opts)
	if err != nil {
		return err
	}
	switch kind {
	case tableColumnBlobString:
		t.Value = &TableColumn_StringBlob{StringBlob: blk}
	case tableColumnBlobBytes:
		t.Value = &TableColumn_BytesBlob{BytesBlob: blk}
	case tableColumnBlobJson:
		t.Value = &TableColumn_JsonBlob{JsonBlob: blk}
	default:
		return errors.Errorf("unsupported table column blob kind %d", kind)
	}
	return nil
}

func (t *TableColumn) blobCtor(kind tableColumnBlobKind) block.SubBlockCtor {
	return func(create bool) block.SubBlock {
		var v *blob.Blob
		switch kind {
		case tableColumnBlobString:
			v = t.GetStringBlob()
		case tableColumnBlobBytes:
			v = t.GetBytesBlob()
		case tableColumnBlobJson:
			v = t.GetJsonBlob()
		}
		if create && v == nil {
			v = &blob.Blob{}
			switch kind {
			case tableColumnBlobString:
				t.Value = &TableColumn_StringBlob{StringBlob: v}
			case tableColumnBlobBytes:
				t.Value = &TableColumn_BytesBlob{BytesBlob: v}
			case tableColumnBlobJson:
				t.Value = &TableColumn_JsonBlob{JsonBlob: v}
			}
		}
		return v
	}
}

func encodeJsonWrapper(ctx context.Context, v sql.JSONWrapper) (string, error) {
	if sv, ok := v.(interface{ JSONString() (string, error) }); ok {
		return sv.JSONString()
	}
	if sv, ok := v.(interface{ String() string }); ok {
		return sv.String(), nil
	}
	return "", errors.Errorf("unsupported json wrapper type %T", v)
}

type tableColumnBlobKind uint8

const (
	tableColumnBlobString tableColumnBlobKind = iota + 1
	tableColumnBlobBytes
	tableColumnBlobJson
)

// _ is a type assertion
var (
	_ block.Block              = (*TableColumn)(nil)
	_ block.BlockWithSubBlocks = (*TableColumn)(nil)
	_ block.SubBlock           = (*TableColumn)(nil)
)
