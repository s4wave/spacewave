//go:build !js

package drivebench

import (
	"github.com/aperturerobotics/fastjson"
)

// OperationShape is the compact per-run storage/frontier summary derived from
// runtime trace tasks and numeric trace-log payloads. Raw runtime.trace remains
// the replay source. This projection is the comparison surface for plan
// ranking.
type OperationShape struct {
	// Operations lists the operation summaries.
	Operations []OperationSummary `json:"operations,omitempty"`
}

// MarshalOperationShapeValue encodes an operation-shape projection in an existing JSON arena.
func MarshalOperationShapeValue(arena *fastjson.Arena, shape OperationShape) *fastjson.Value {
	obj := arena.NewObject()
	if len(shape.Operations) != 0 {
		arr := arena.NewArray()
		for _, op := range shape.Operations {
			arr.SetArrayItem(len(arr.GetArray()), marshalOperationSummaryValue(arena, op))
		}
		obj.Set("operations", arr)
	}
	return obj
}
