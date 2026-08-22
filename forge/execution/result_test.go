package forge_execution

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	net_hash "github.com/s4wave/spacewave/net/hash"
)

// TestComputeExecutionOutputsValueMismatch pins that executions disagreeing on
// an output value fail instead of silently returning one side's outputs.
func TestComputeExecutionOutputsValueMismatch(t *testing.T) {
	outputs := []*forge_target.Output{{
		Name:       "store",
		OutputType: forge_target.OutputType_OutputType_EXEC,
		ExecOutput: "store",
	}}
	execOutputVals := []forge_value.ValueSlice{
		{{Name: "store", ValueType: forge_value.ValueType_ValueType_BLOCK_REF, BlockRef: &block.BlockRef{Hash: &net_hash.Hash{HashType: net_hash.HashType_HashType_SHA256, Hash: []byte{1}}}}},
		{{Name: "store", ValueType: forge_value.ValueType_ValueType_BLOCK_REF, BlockRef: &block.BlockRef{Hash: &net_hash.Hash{HashType: net_hash.HashType_HashType_SHA256, Hash: []byte{2}}}}},
	}
	if _, err := ComputeExecutionOutputs(outputs, execOutputVals, false); err == nil {
		t.Fatal("expected divergent execution outputs to return an error")
	}
}
