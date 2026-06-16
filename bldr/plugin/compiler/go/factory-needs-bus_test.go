//go:build !js

package bldr_plugin_compiler_go

import (
	"go/token"
	"go/types"
	"testing"
)

// newFactorySig builds a NewFactory-like signature for arity classification.
func newFactorySig(variadic bool, params ...types.Type) *types.Signature {
	vars := make([]*types.Var, len(params))
	for i, p := range params {
		vars[i] = types.NewParam(token.NoPos, nil, "", p)
	}
	return types.NewSignatureType(nil, nil, nil, types.NewTuple(vars...), nil, variadic)
}

// TestFactoryNeedsBus pins the factory arity rule, including the variadic option
// seam that broke wasm plugin codegen when NewFactory(bus, ...Option) reported
// arity 2.
func TestFactoryNeedsBus(t *testing.T) {
	busType := types.Typ[types.Int]                   // stands in for bus.Bus
	optsSlice := types.NewSlice(types.Typ[types.Int]) // stands in for []Option

	tests := []struct {
		name    string
		sig     *types.Signature
		wantBus bool
		wantErr bool
	}{
		{name: "no args", sig: newFactorySig(false), wantBus: false},
		{name: "bus only", sig: newFactorySig(false, busType), wantBus: true},
		{name: "variadic opts only", sig: newFactorySig(true, optsSlice), wantBus: false},
		{name: "bus plus variadic opts", sig: newFactorySig(true, busType, optsSlice), wantBus: true},
		{name: "two required args", sig: newFactorySig(false, busType, busType), wantErr: true},
		{name: "two required plus variadic", sig: newFactorySig(true, busType, busType, optsSlice), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			needsBus, err := FactoryNeedsBus("github.com/example/factory", tc.sig)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if needsBus != tc.wantBus {
				t.Fatalf("needsBus = %v, want %v", needsBus, tc.wantBus)
			}
		})
	}
}
