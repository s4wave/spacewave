package task_tx

import (
	"testing"

	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

func TestTxRetryValidateRejectsOutputs(t *testing.T) {
	tx := &TxRetry{NextInputs: &forge_target.ValueSet{
		Outputs: forge_value.ValueSlice{forge_value.NewValue("output")},
	}}
	if err := tx.Validate(); err == nil {
		t.Fatal("retry accepted output values")
	}
}

func TestTxRetryValidateAcceptsNamedInputs(t *testing.T) {
	tx := &TxRetry{NextInputs: &forge_target.ValueSet{
		Inputs: forge_value.ValueSlice{forge_value.NewValueWithWorldObjectSnapshot(
			"continuation",
			&forge_value.WorldObjectSnapshot{Key: "session"},
		)},
	}}
	if err := tx.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSameInputsUsesValueContent(t *testing.T) {
	left := forge_value.ValueSlice{forge_value.NewValue("continuation")}
	right := forge_value.ValueSlice{forge_value.NewValue("continuation")}
	if !sameInputs(left, right) {
		t.Fatal("equal named inputs did not compare equal")
	}
	right[0].Name = "other"
	if sameInputs(left, right) {
		t.Fatal("different named inputs compared equal")
	}
}
