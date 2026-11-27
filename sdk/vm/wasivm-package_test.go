package s4wave_vm

import (
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// TestResolveWasiVmPackages checks that a plan lists every package after its
// dependencies, in key order, and ends a dependency cycle where it is found.
func TestResolveWasiVmPackages(t *testing.T) {
	// Start a World with the package operation registered.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	opc := world.NewLookupOpController("test-wasivm-ops", tb.EngineID, world.NewSchema(WasivmTypes).LookupOp)
	if _, err := tb.Bus.AddController(ctx, opc, nil); err != nil {
		t.Fatal(err)
	}
	ws := world.NewEngineWorldState(tb.Engine, true)

	// Create the packages and their dependency edges. htop needs ncurses and
	// musl-extra. ncurses and readline depend on each other.
	key := func(name string) string { return WasiVmPackageKey("test/"+name, "1") }
	deps := map[string][]string{
		"htop":       {"ncurses", "musl-extra"},
		"ncurses":    {"readline"},
		"readline":   {"ncurses"},
		"musl-extra": nil,
	}
	for name := range deps {
		pkg := &WasiVmPackage{Name: "test/" + name, Version: "1"}
		op := NewCreateWasiVmPackageOp(key(name), pkg, time.Unix(1, 0))
		if _, _, err := ws.ApplyWorldOp(ctx, op, ""); err != nil {
			t.Fatal(err)
		}
	}
	for name, names := range deps {
		for _, dep := range names {
			gq := world.NewGraphQuadWithKeys(key(name), string(PredWasiVmPackageDepends), key(dep), "")
			if err := ws.SetGraphQuad(ctx, gq); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Resolve htop's plan.
	plan, err := ResolveWasiVmPackages(ctx, ws, []string{key("htop")})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{key("musl-extra"), key("readline"), key("ncurses"), key("htop")}
	if !slices.Equal(plan, want) {
		t.Fatalf("plan %v, want %v", plan, want)
	}
}
