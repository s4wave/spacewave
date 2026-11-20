package forge_task

import (
	"context"
	"encoding/binary"
	"slices"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/hash"
)

// PredTaskToReuseKey is the graph predicate linking a Task whose Target reuses
// outputs to its reuse key, so a later Task with the same key finds it through
// the graph index instead of a scan.
const PredTaskToReuseKey = quad.IRI("forge/task-reuse-key")

// reuseKeyPrefix prefixes the IRI a reuse key is stored under.
const reuseKeyPrefix = "forge/task-reuse/"

// NewReuseKey returns the reuse key of a Target run with the given inputs, or
// "" when the Target does not reuse outputs. The key covers the Exec and the
// Outputs of the Target and every input value except those the Exec names in
// ReuseIgnoreInputs. It leaves out the Target's input declarations, which name
// where a value came from, such as an upstream Task, and not what it is.
func NewReuseKey(tgt *forge_target.Target, inputs forge_value.ValueSlice) (string, error) {
	// Only a Target that declares its outputs reusable has a key.
	exec := tgt.GetExec()
	if !exec.GetReuseOutputs() {
		return "", nil
	}

	// Keep the inputs that affect the outputs, in name order.
	ignore := make(map[string]struct{}, len(exec.GetReuseIgnoreInputs()))
	for _, name := range exec.GetReuseIgnoreInputs() {
		ignore[name] = struct{}{}
	}
	keyed := make([]*forge_value.Value, 0, len(inputs))
	for _, input := range inputs {
		if _, skip := ignore[input.GetName()]; !skip {
			keyed = append(keyed, input)
		}
	}
	slices.SortFunc(keyed, func(a, b *forge_value.Value) int {
		return strings.Compare(a.GetName(), b.GetName())
	})

	// Encode the Exec and Outputs, then the keyed inputs.
	tgtPart, err := (&forge_target.Target{Exec: exec, Outputs: tgt.GetOutputs()}).MarshalVT()
	if err != nil {
		return "", err
	}
	valPart, err := (&forge_target.ValueSet{Inputs: keyed}).MarshalVT()
	if err != nil {
		return "", err
	}

	// Hash the parts, length-prefixing the first so the split is unambiguous.
	data := binary.AppendUvarint(nil, uint64(len(tgtPart)))
	data = append(data, tgtPart...)
	data = append(data, valPart...)
	h, err := hash.Sum(hash.RecommendedHashType, data)
	if err != nil {
		return "", err
	}
	return h.MarshalString(), nil
}

// NewTaskToReuseKeyQuad creates a quad linking a Task to its reuse key.
func NewTaskToReuseKeyQuad(taskKey, reuseKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		taskKey,
		PredTaskToReuseKey.String(),
		reuseKeyPrefix+reuseKey,
		"",
	)
}

// ListTasksWithReuseKey lists up to limit Tasks recorded with the reuse key.
func ListTasksWithReuseKey(ctx context.Context, ws world.WorldState, reuseKey string, limit uint32) ([]string, error) {
	// Look up the Tasks linked to the key through the graph index.
	gqs, err := ws.LookupGraphQuads(ctx, NewTaskToReuseKeyQuad("", reuseKey), limit)
	if err != nil {
		return nil, err
	}

	// Return each quad's subject as an object key.
	keys := make([]string, 0, len(gqs))
	for _, gq := range gqs {
		key, err := world.GraphValueToKey(gq.GetSubject())
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// CheckTaskReuseKey reports whether the Task is recorded with the reuse key.
func CheckTaskReuseKey(ctx context.Context, ws world.WorldState, taskKey, reuseKey string) (bool, error) {
	gqs, err := ws.LookupGraphQuads(ctx, NewTaskToReuseKeyQuad(taskKey, reuseKey), 1)
	return len(gqs) != 0, err
}
