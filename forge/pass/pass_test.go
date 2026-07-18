package forge_pass

import (
	"testing"

	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

func TestComputeOutputsWithStatesPromotesUnanimousFailedOutputs(t *testing.T) {
	outputs := []*forge_target.Output{{
		Name:       "continuation",
		OutputType: forge_target.OutputType_OutputType_EXEC,
		ExecOutput: "continuation",
	}}
	value := forge_value.NewValueWithWorldObjectSnapshot(
		"continuation",
		&forge_value.WorldObjectSnapshot{Key: "session"},
	)
	states := []*ExecState{
		failedExecState("exec-a", value),
		failedExecState("exec-b", value),
	}
	promoted, err := ComputeOutputsWithStates(outputs, states, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(promoted) != 1 || !promoted[0].Equals(value) {
		t.Fatalf("promoted outputs: %+v", promoted)
	}
}

func TestComputeOutputsWithStatesRejectsDivergentFailedOutputs(t *testing.T) {
	outputs := []*forge_target.Output{{
		Name:       "continuation",
		OutputType: forge_target.OutputType_OutputType_EXEC,
		ExecOutput: "continuation",
	}}
	states := []*ExecState{
		failedExecState("exec-a", forge_value.NewValueWithWorldObjectSnapshot(
			"continuation", &forge_value.WorldObjectSnapshot{Key: "session-a"},
		)),
		failedExecState("exec-b", forge_value.NewValueWithWorldObjectSnapshot(
			"continuation", &forge_value.WorldObjectSnapshot{Key: "session-b"},
		)),
	}
	if _, err := ComputeOutputsWithStates(outputs, states, 2); err == nil {
		t.Fatal("divergent failed outputs were promoted")
	}
}

func failedExecState(key string, value *forge_value.Value) *ExecState {
	return &ExecState{
		ObjectKey:      key,
		ExecutionState: forge_execution.State_ExecutionState_COMPLETE,
		ValueSet:       &forge_target.ValueSet{Outputs: forge_value.ValueSlice{value}},
		Result:         forge_value.NewResultWithError(assertionError{}),
	}
}

type assertionError struct{}

func (assertionError) Error() string { return "failed" }
