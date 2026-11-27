//go:build !js

package drivebench

import (
	"strconv"

	"github.com/aperturerobotics/fastjson"
)

// OperationSummary groups task timing and recovered numeric log payloads under a
// stable operation name, such as block-write or space-replay.
type OperationSummary struct {
	// Name is the operation name.
	Name string `json:"name"`
	// Count is the number of tasks of the operation.
	Count int `json:"count,omitempty"`
	// TotalUs is their total duration in microseconds.
	TotalUs int64 `json:"totalUs,omitempty"`
	// MaxUs is the longest task duration in microseconds.
	MaxUs int64 `json:"maxUs,omitempty"`
	// LogCount counts the trace logs recovered for the operation.
	LogCount int `json:"logCount,omitempty"`
	// Fields summarizes the numeric fields of those logs.
	Fields []OperationField `json:"fields,omitempty"`
}

func marshalOperationSummaryValue(arena *fastjson.Arena, op OperationSummary) *fastjson.Value {
	// Encode the operation's name and recorded timing and log counts.
	obj := arena.NewObject()
	obj.Set("name", arena.NewString(op.Name))
	if op.Count != 0 {
		obj.Set("count", arena.NewNumberInt(op.Count))
	}
	if op.TotalUs != 0 {
		obj.Set("totalUs", arena.NewNumberString(strconv.FormatInt(op.TotalUs, 10)))
	}
	if op.MaxUs != 0 {
		obj.Set("maxUs", arena.NewNumberString(strconv.FormatInt(op.MaxUs, 10)))
	}
	if op.LogCount != 0 {
		obj.Set("logCount", arena.NewNumberInt(op.LogCount))
	}

	// Include the operation's numeric field summaries when available.
	if len(op.Fields) != 0 {
		arr := arena.NewArray()
		for _, field := range op.Fields {
			arr.SetArrayItem(len(arr.GetArray()), marshalOperationFieldValue(arena, field))
		}
		obj.Set("fields", arr)
	}
	return obj
}
