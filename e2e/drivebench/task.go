//go:build !js

package drivebench

import (
	"strconv"

	"github.com/aperturerobotics/fastjson"
)

// Task is an aggregate runtime-trace task summary keyed by Go user task type.
type Task struct {
	// Type is the Go user task type.
	Type string `json:"type"`
	// Count is the number of tasks of the type.
	Count int `json:"count"`
	// TotalUs is their total duration in microseconds.
	TotalUs int64 `json:"totalUs"`
	// MaxUs is the longest one in microseconds.
	MaxUs int64 `json:"maxUs"`
}

func marshalTasksValue(arena *fastjson.Arena, tasks []Task) *fastjson.Value {
	arr := arena.NewArray()
	for _, task := range tasks {
		obj := arena.NewObject()
		obj.Set("type", arena.NewString(task.Type))
		obj.Set("count", arena.NewNumberInt(task.Count))
		obj.Set("totalUs", arena.NewNumberString(strconv.FormatInt(task.TotalUs, 10)))
		obj.Set("maxUs", arena.NewNumberString(strconv.FormatInt(task.MaxUs, 10)))
		arr.SetArrayItem(len(arr.GetArray()), obj)
	}
	return arr
}
