//go:build !js

package drivebench

import (
	"github.com/aperturerobotics/fastjson"
)

// Phase is one browser-observed quickstart phase. The timestamps use the page
// performance.now timebase, matching the quickstart timing object published by
// the app while the bench is running.
type Phase struct {
	// Name is the phase name.
	Name string `json:"name"`
	// StartedMs is when the phase started.
	StartedMs int `json:"startedMs"`
	// FinishedMs is when the phase finished, if it did.
	FinishedMs *int `json:"finishedMs,omitempty"`
	// ElapsedMs is the phase duration, if it finished.
	ElapsedMs *int `json:"elapsedMs,omitempty"`
	// Error is the error the phase ended with, if any.
	Error string `json:"error,omitempty"`
}

func parsePhases(raw any) []Phase {
	// Require a phase array before parsing quickstart timing.
	items, ok := raw.([]any)
	if !ok {
		return nil
	}

	// Recover named phases with their timing and error details.
	phases := make([]Phase, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		if name == "" {
			continue
		}
		phase := Phase{
			Name:       name,
			StartedMs:  intValue(m["startedMs"]),
			FinishedMs: optionalInt(m, "finishedMs"),
			ElapsedMs:  optionalInt(m, "elapsedMs"),
		}
		phase.Error, _ = m["error"].(string)
		phases = append(phases, phase)
	}
	return phases
}

func marshalPhasesValue(arena *fastjson.Arena, phases []Phase) *fastjson.Value {
	arr := arena.NewArray()
	for _, phase := range phases {
		obj := arena.NewObject()
		obj.Set("name", arena.NewString(phase.Name))
		obj.Set("startedMs", arena.NewNumberInt(phase.StartedMs))
		setOptionalIntJSONField(arena, obj, "finishedMs", phase.FinishedMs)
		setOptionalIntJSONField(arena, obj, "elapsedMs", phase.ElapsedMs)
		setOmitEmptyStringJSONField(arena, obj, "error", phase.Error)
		arr.SetArrayItem(len(arr.GetArray()), obj)
	}
	return arr
}
