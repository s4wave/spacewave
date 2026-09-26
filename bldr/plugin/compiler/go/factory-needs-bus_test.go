//go:build !js

package bldr_plugin_compiler_go

import (
	"testing"
)

// TestFactoryNeedsBus pins the factory arity rule, including the variadic option
// seam that broke wasm plugin codegen when NewFactory(bus, ...Option) reported
// arity 2.
func TestFactoryNeedsBus(t *testing.T) {
	tests := []struct {
		name    string
		sig     *Constructor
		wantBus bool
		wantErr bool
	}{
		{name: "no args", sig: &Constructor{IsFunction: true}, wantBus: false},
		{name: "bus only", sig: &Constructor{IsFunction: true, Parameters: 1}, wantBus: true},
		{name: "variadic opts only", sig: &Constructor{IsFunction: true, Parameters: 1, Variadic: true}, wantBus: false},
		{name: "bus plus variadic opts", sig: &Constructor{IsFunction: true, Parameters: 2, Variadic: true}, wantBus: true},
		{name: "two required args", sig: &Constructor{IsFunction: true, Parameters: 2}, wantErr: true},
		{name: "two required plus variadic", sig: &Constructor{IsFunction: true, Parameters: 3, Variadic: true}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			needsBus, err := tc.sig.NeedsBus("github.com/example/factory")
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
