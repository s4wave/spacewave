package mysql

import (
	"bytes"
	"context"
	"time"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// TableColumnMaxSize is the maximum size of the data field, if larger than
// this size, data will be stored in a block referenced by "data_ref".
const TableColumnMaxSize = 2e4 // 20kb

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
	bcs.ClearAllRefs()
	bcs.SetBlock(ntc, true)

	switch v := col.(type) {
	case nil:
	case bool:
		ntc.Value = &TableColumn_BoolValue{BoolValue: v}
	case int:
		ntc.Value = &TableColumn_IntValue{IntValue: int64(v)}
	case int8:
		ntc.Value = &TableColumn_IntValue{IntValue: int64(v)}
	case int16:
		ntc.Value = &TableColumn_IntValue{IntValue: int64(v)}
	case int32:
		ntc.Value = &TableColumn_IntValue{IntValue: int64(v)}
	case int64:
		ntc.Value = &TableColumn_IntValue{IntValue: v}
	case uint:
		ntc.Value = &TableColumn_UintValue{UintValue: uint64(v)}
	case uint8:
		ntc.Value = &TableColumn_UintValue{UintValue: uint64(v)}
	case uint16:
		ntc.Value = &TableColumn_UintValue{UintValue: uint64(v)}
	case uint32:
		ntc.Value = &TableColumn_UintValue{UintValue: uint64(v)}
	case uint64:
		ntc.Value = &TableColumn_UintValue{UintValue: v}
	case float32:
		ntc.Value = &TableColumn_FloatValue{FloatValue: float64(v)}
	case float64:
		ntc.Value = &TableColumn_FloatValue{FloatValue: v}
	case string:
		if err := ntc.setBlob(ctx, bcs, opts, 6, []byte(v), tableColumnBlobString); err != nil {
			return nil, err
		}
	case []byte:
		if err := ntc.setBlob(ctx, bcs, opts, 7, v, tableColumnBlobBytes); err != nil {
			return nil, err
		}
	case time.Time:
		nanos := v.Nanosecond()
		ntc.Value = &TableColumn_TimestampValue{
			TimestampValue: &TableTimestamp{
				UnixSeconds: v.Unix(),
				Nanos:       int32(nanos), // #nosec G115 -- Nanosecond is in [0, 999999999].
			},
		}
	case types.Timespan:
		ntc.Value = &TableColumn_TimespanMicros{TimespanMicros: int64(v)}
	case sql.JSONWrapper:
		jsonString, err := encodeJsonWrapper(ctx, v)
		if err != nil {
			return nil, err
		}
		if err := ntc.setBlob(ctx, bcs, opts, 8, []byte(jsonString), tableColumnBlobJson); err != nil {
			return nil, err
		}
	default:
		return nil, errors.Errorf("unsupported table column type %T", col)
	}

	return ntc, nil
}

// IsNil returns if the object is nil.
func (t *TableColumn) IsNil() bool {
	return t == nil
}

// IsEmpty checks if the table column is empty.
func (t *TableColumn) IsEmpty() bool {
	return t.GetValue() == nil
}

// FetchSqlColumn converts the row back into a sql column.
func (t *TableColumn) FetchSqlColumn(ctx context.Context, bcs *block.Cursor) (any, error) {
	if t == nil || bcs == nil {
		// treat nil object or cursor as nil column
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
		data, err := blob.FetchToBytes(ctx, bcs.FollowSubBlock(6))
		if err != nil {
			return nil, err
		}
		return string(data), nil
	case *TableColumn_BytesBlob:
		return blob.FetchToBytes(ctx, bcs.FollowSubBlock(7))
	case *TableColumn_JsonBlob:
		data, err := blob.FetchToBytes(ctx, bcs.FollowSubBlock(8))
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
	default:
		return nil, errors.Errorf("unsupported table column value %T", v)
	}
}

// Validate performs cursory validation of the table column.
func (t *TableColumn) Validate() error {
	switch t.GetValue().(type) {
	case nil:
	case *TableColumn_BoolValue:
	case *TableColumn_IntValue:
	case *TableColumn_UintValue:
	case *TableColumn_FloatValue:
	case *TableColumn_TimestampValue:
	case *TableColumn_TimespanMicros:
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

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (t *TableColumn) MarshalBlock() ([]byte, error) {
	return t.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (t *TableColumn) UnmarshalBlock(data []byte) error {
	return t.UnmarshalVT(data)
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
	_ block.Block              = ((*TableColumn)(nil))
	_ block.BlockWithSubBlocks = ((*TableColumn)(nil))
	_ block.SubBlock           = ((*TableColumn)(nil))
)
