package packsweep

import (
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/aperturerobotics/fastjson"
)

// JSON describes the header for a person or a script.
func (h *Head) JSON(a *fastjson.Arena) *fastjson.Value {
	return jsonObject(a,
		jsonField{"cycle", a.NewNumberInt(h.Cycle)},
		jsonField{"snapshotAt", jsonTime(a, h.SnapshotAt)},
		jsonField{"snapshotPacks", a.NewNumberInt(h.SnapshotPacks)},
		jsonField{"catalogued", jsonBool(a, h.Catalogued)},
		jsonField{"roots", jsonStrings(a, h.Roots)},
		jsonField{"logPosition", a.NewString(h.LogPosition)},
		jsonField{"seeded", jsonBool(a, h.Seeded)},
		jsonField{"walked", jsonBool(a, h.Walked)},
		jsonField{"live", jsonUint(a, h.Live)},
		jsonField{"closure", jsonUint(a, h.Closure)},
		jsonField{"holes", JSONCounts(a, h.Holes)},
		jsonField{"judgedAt", jsonTime(a, h.JudgedAt)},
	)
}

// JSON describes the judgment, one line of the report.
func (j *Judgment) JSON(a *fastjson.Arena) *fastjson.Value {
	return jsonObject(a,
		jsonField{"id", a.NewString(j.ID)},
		jsonField{"verdict", a.NewString(j.Verdict.String())},
		jsonField{"blocks", jsonUint(a, j.Blocks)},
		jsonField{"liveBlocks", jsonUint(a, j.LiveBlocks)},
		jsonField{"valueBytes", jsonUint(a, j.ValueBytes)},
		jsonField{"liveBytes", jsonUint(a, j.LiveBytes)},
		jsonField{"size", jsonUint(a, j.Size)},
		jsonField{"created", jsonTime(a, j.Created)},
	)
}

// JSON describes the log pull.
func (p *LogPull) JSON(a *fastjson.Arena) *fastjson.Value {
	return jsonObject(a,
		jsonField{"start", a.NewString(p.Start)},
		jsonField{"position", a.NewString(p.Position)},
		jsonField{"lines", jsonUint(a, p.Lines)},
		jsonField{"seeds", jsonUint(a, p.Seeds)},
		jsonField{"gap", a.NewString(p.Gap)},
	)
}

// JSON describes the summary.
func (s *Summary) JSON(a *fastjson.Arena) *fastjson.Value {
	return jsonObject(a,
		jsonField{"packs", JSONCounts(a, s.Packs)},
		jsonField{"bytes", JSONCounts(a, s.Bytes)},
		jsonField{"deadBytesInLow", jsonUint(a, s.DeadBytesInLow)},
	)
}

// JSON describes the snapshot's progress.
func (p *SnapshotProgress) JSON(a *fastjson.Arena) *fastjson.Value {
	return jsonObject(a,
		jsonField{"packs", a.NewNumberInt(p.Packs)},
		jsonField{"catalogued", a.NewNumberInt(p.Catalogued)},
		jsonField{"skipped", a.NewNumberInt(p.Skipped)},
		jsonField{"bytes", jsonUint(a, p.Bytes)},
	)
}

// jsonField is one member of an object.
type jsonField struct {
	// key names the member.
	key string
	// value is its value.
	value *fastjson.Value
}

// jsonObject is an object of the fields, in their order.
func jsonObject(a *fastjson.Arena, fields ...jsonField) *fastjson.Value {
	o := a.NewObject()
	for _, f := range fields {
		o.Set(f.key, f.value)
	}
	return o
}

// jsonStrings is an array of strings.
func jsonStrings(a *fastjson.Arena, values []string) *fastjson.Value {
	arr := a.NewArray()
	for i, v := range values {
		arr.SetArrayItem(i, a.NewString(v))
	}
	return arr
}

// JSONCounts describes counts by name, in name order.
func JSONCounts(a *fastjson.Arena, counts map[string]uint64) *fastjson.Value {
	o := a.NewObject()
	for _, k := range slices.Sorted(maps.Keys(counts)) {
		o.Set(k, jsonUint(a, counts[k]))
	}
	return o
}

// jsonUint is an unsigned number, exact past 2^53.
func jsonUint(a *fastjson.Arena, v uint64) *fastjson.Value {
	return a.NewNumberString(strconv.FormatUint(v, 10))
}

// jsonBool is a boolean.
func jsonBool(a *fastjson.Arena, v bool) *fastjson.Value {
	if v {
		return a.NewTrue()
	}
	return a.NewFalse()
}

// jsonTime is an RFC 3339 time, or null for the zero time.
func jsonTime(a *fastjson.Arena, t time.Time) *fastjson.Value {
	if t.IsZero() {
		return a.NewNull()
	}
	return a.NewString(t.UTC().Format(time.RFC3339Nano))
}

// JSON describes the quarantine.
func (r *QuarantineResult) JSON(a *fastjson.Arena) *fastjson.Value {
	return jsonObject(a,
		jsonField{"blocked", a.NewString(r.Blocked)},
		jsonField{"quarantined", jsonUint(a, r.Quarantined)},
		jsonField{"live", jsonUint(a, r.Live)},
		jsonField{"unmoved", jsonUint(a, r.Unmoved)},
	)
}

// JSON describes the recheck.
func (r *RecheckResult) JSON(a *fastjson.Arena) *fastjson.Value {
	return jsonObject(a,
		jsonField{"blocked", a.NewString(r.Blocked)},
		jsonField{"passed", jsonUint(a, r.Passed)},
		jsonField{"lifted", jsonUint(a, r.Lifted)},
		jsonField{"live", jsonUint(a, r.Live)},
		jsonField{"waiting", JSONCounts(a, r.Waiting)},
	)
}
