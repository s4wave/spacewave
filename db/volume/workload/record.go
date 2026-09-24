package workload

import (
	"context"
	"encoding/hex"
	"runtime/trace"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// Record is one logical storage operation.
type Record struct {
	// Op names the operation.
	Op Op
	// ID identifies the transaction, iterator, read scope, or batch.
	ID uint64
	// Parent identifies the transaction that owns an iterator.
	Parent uint64
	// Size is the operation's length, count, or result, as its Op documents.
	Size int64
	// Key is the key, key prefix, or binary block reference.
	Key []byte
}

// ParseRecord decodes a record from the text AppendText produced.
func ParseRecord(msg string) (Record, error) {
	// Split the five space-separated fields.
	fields := strings.Split(msg, " ")
	if len(fields) != 5 {
		return Record{}, errors.Errorf("workload record: want 5 fields, have %d: %q", len(fields), msg)
	}

	// Decode the numeric fields and the hexadecimal key.
	rec := Record{Op: Op(fields[0])}
	var err error
	if rec.ID, err = strconv.ParseUint(fields[1], 10, 64); err != nil {
		return Record{}, errors.Wrap(err, "workload record id")
	}
	if rec.Parent, err = strconv.ParseUint(fields[2], 10, 64); err != nil {
		return Record{}, errors.Wrap(err, "workload record parent")
	}
	if rec.Size, err = strconv.ParseInt(fields[3], 10, 64); err != nil {
		return Record{}, errors.Wrap(err, "workload record size")
	}
	if rec.Key, err = hex.DecodeString(fields[4]); err != nil {
		return Record{}, errors.Wrap(err, "workload record key")
	}
	return rec, nil
}

// Log emits the record to the running execution trace, if any.
func (r Record) Log(ctx context.Context) {
	if trace.IsEnabled() {
		trace.Log(ctx, Category, string(r.AppendText(nil)))
	}
}

// AppendText appends the record's single-line text form, which ParseRecord
// decodes: op, id, parent, size, and the hexadecimal key.
func (r Record) AppendText(b []byte) []byte {
	b = append(b, r.Op...)
	b = append(b, ' ')
	b = strconv.AppendUint(b, r.ID, 10)
	b = append(b, ' ')
	b = strconv.AppendUint(b, r.Parent, 10)
	b = append(b, ' ')
	b = strconv.AppendInt(b, r.Size, 10)
	b = append(b, ' ')
	return hex.AppendEncode(b, r.Key)
}
