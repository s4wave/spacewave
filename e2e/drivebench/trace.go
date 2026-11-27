//go:build !js

package drivebench

import (
	"github.com/aperturerobotics/fastjson"
)

// Trace summarizes the captured runtime trace and the paths of the raw trace
// plus its tracetool extraction.
type Trace struct {
	// Bytes is the size of the raw runtime trace.
	Bytes int `json:"bytes"`
	// RuntimeTracePath is the path of the raw runtime trace.
	RuntimeTracePath string `json:"runtimeTracePath"`
	// TracetoolPath is the path of the tracetool extraction.
	TracetoolPath string `json:"tracetoolPath,omitempty"`
	// UserTasks counts the Go user tasks in the trace.
	UserTasks int `json:"userTasks,omitempty"`
	// UserRegions counts the Go user regions in the trace.
	UserRegions int `json:"userRegions,omitempty"`
	// UserLogs counts the Go user log events in the trace.
	UserLogs int `json:"userLogs,omitempty"`
	// Tasks summarizes the user tasks by type.
	Tasks []Task `json:"tasks,omitempty"`
}

func marshalTraceValue(arena *fastjson.Arena, trace Trace) *fastjson.Value {
	// Encode the raw trace's size and artifact path.
	obj := arena.NewObject()
	obj.Set("bytes", arena.NewNumberInt(trace.Bytes))
	obj.Set("runtimeTracePath", arena.NewString(trace.RuntimeTracePath))

	// A cell that captured the raw trace without summarizing it has no
	// counts to report, and the tracetool path is what says whether
	// summarization ran. Key the whole summary block on the tracetool path
	// rather than on the counts: a summarized cell that legitimately saw no
	// user regions must serialize that zero, or a consumer cannot tell a
	// measured zero from an unsummarized cell.
	if trace.TracetoolPath != "" {
		obj.Set("tracetoolPath", arena.NewString(trace.TracetoolPath))
		obj.Set("userTasks", arena.NewNumberInt(trace.UserTasks))
		obj.Set("userRegions", arena.NewNumberInt(trace.UserRegions))
		obj.Set("userLogs", arena.NewNumberInt(trace.UserLogs))
		obj.Set("tasks", marshalTasksValue(arena, trace.Tasks))
	}
	return obj
}
