package task_controller

import (
	"testing"

	task_tx "github.com/s4wave/spacewave/forge/task/tx"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

func TestBuildUpdateInputValueSetForTargetOnlyUpdate(t *testing.T) {
	storedInputs := forge_value.ValueSlice{forge_value.NewValue("scheduler-input")}
	valueSet := buildUpdateInputValueSet(storedInputs, nil, nil, nil)

	tx := task_tx.NewTxUpdateInputs("task")
	tx.TxUpdateInputs.UpdateTarget = true
	tx.TxUpdateInputs.ValueSet = valueSet
	if err := tx.Validate(); err != nil {
		t.Fatalf("target-only update with stored inputs is invalid: %v", err)
	}
	if len(valueSet.GetInputs()) != 1 || valueSet.GetInputs()[0].GetName() != "scheduler-input" {
		t.Fatalf("value set inputs = %v, want stored input", valueSet.GetInputs())
	}
}
