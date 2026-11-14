//go:build !js

package drivebench

import (
	"github.com/aperturerobotics/fastjson"
)

// StartupMark is one startup-boundary mark observed on the page performance
// timeline, emitted by markStartupBoundary. StartMs is the mark's
// performance.now time in milliseconds from navigation start, so marks share
// the timebase of the bundled milestones and quickstart phases. The
// pre-quickstart startup gap is attributed by reading the interval between
// adjacent marks: the gap belongs to whichever mark transition spans it.
type StartupMark struct {
	// Label is the mark name without its prefix.
	Label string `json:"label"`
	// Sequence is the boundary order markStartupBoundary assigned.
	Sequence int `json:"sequence,omitempty"`
	// StartMs is the mark time from navigation start.
	StartMs int `json:"startMs"`
	// Phase is the startup phase the boundary belongs to.
	Phase string `json:"phase,omitempty"`
	// Mode is the boot mode the boundary recorded.
	Mode string `json:"mode,omitempty"`
	// Source names what emitted the boundary.
	Source string `json:"source,omitempty"`
	// Detail is the full detail stored on the mark.
	Detail map[string]any `json:"detail,omitempty"`
}

// StartupMarksScript reads the page startup-mark timeline into a JSON-able
// array ordered by performance.now start time. Each entry carries the mark
// label, its start time in milliseconds from navigation start, and the boundary
// detail markStartupBoundary stored on the performance mark (sequence, phase,
// mode, source). Both Drive benches evaluate it on the page after content-ready
// so the captured marks span the full boot-to-content window.
const StartupMarksScript = `() => {
  const prefix = 'spacewave.startup.'
  return performance.getEntriesByType('mark')
    .filter((m) => m.name.startsWith(prefix))
    .map((m) => ({
      label: m.name.slice(prefix.length),
      startMs: Math.round(m.startTime),
      sequence: m.detail?.sequence ?? 0,
      phase: m.detail?.phase ?? null,
      mode: m.detail?.mode ?? null,
      source: m.detail?.source ?? null,
      detail: m.detail ?? {},
    }))
}`

// ParseStartupMarks parses the page startup-mark timeline captured with
// StartupMarksScript. Marks arrive ordered by performance.now start time; the
// parser preserves that order so the pre-quickstart gap reads as the interval
// between adjacent marks. Entries without a label are skipped.
func ParseStartupMarks(raw any) []StartupMark {
	// Require a startup-mark array before parsing the browser timeline.
	items, ok := raw.([]any)
	if !ok {
		return nil
	}

	// Recover labeled startup marks in their captured order.
	marks := make([]StartupMark, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		label, _ := m["label"].(string)
		if label == "" {
			continue
		}
		mark := StartupMark{
			Label:    label,
			Sequence: intValue(m["sequence"]),
			StartMs:  intValue(m["startMs"]),
		}
		mark.Phase, _ = m["phase"].(string)
		mark.Mode, _ = m["mode"].(string)
		mark.Source, _ = m["source"].(string)
		if detail, ok := m["detail"].(map[string]any); ok && len(detail) > 0 {
			mark.Detail = detail
		}
		marks = append(marks, mark)
	}
	return marks
}

func marshalStartupMarksValue(arena *fastjson.Arena, marks []StartupMark) *fastjson.Value {
	arr := arena.NewArray()
	for _, mark := range marks {
		obj := arena.NewObject()
		obj.Set("label", arena.NewString(mark.Label))
		if mark.Sequence != 0 {
			obj.Set("sequence", arena.NewNumberInt(mark.Sequence))
		}
		obj.Set("startMs", arena.NewNumberInt(mark.StartMs))
		setOmitEmptyStringJSONField(arena, obj, "phase", mark.Phase)
		setOmitEmptyStringJSONField(arena, obj, "mode", mark.Mode)
		setOmitEmptyStringJSONField(arena, obj, "source", mark.Source)
		if len(mark.Detail) != 0 {
			obj.Set("detail", marshalJSONMapValue(arena, mark.Detail))
		}
		arr.SetArrayItem(len(arr.GetArray()), obj)
	}
	return arr
}
