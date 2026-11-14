//go:build !js

package drivebench

import (
	"strconv"

	"github.com/aperturerobotics/fastjson"
)

// OperationField summarizes one numeric field recovered from trace logs.
type OperationField struct {
	// Name is the field name.
	Name string `json:"name"`
	// Samples counts the logs carrying the field.
	Samples int `json:"samples"`
	// Sum is the total of its values.
	Sum int64 `json:"sum"`
	// Max is its largest value.
	Max int64 `json:"max"`
	// Last is its last value.
	Last int64 `json:"last"`
}

func marshalOperationFieldValue(arena *fastjson.Arena, field OperationField) *fastjson.Value {
	// Encode the numeric field's sample count and aggregate values.
	obj := arena.NewObject()
	obj.Set("name", arena.NewString(field.Name))
	obj.Set("samples", arena.NewNumberInt(field.Samples))
	obj.Set("sum", arena.NewNumberString(strconv.FormatInt(field.Sum, 10)))
	obj.Set("max", arena.NewNumberString(strconv.FormatInt(field.Max, 10)))
	obj.Set("last", arena.NewNumberString(strconv.FormatInt(field.Last, 10)))
	return obj
}
