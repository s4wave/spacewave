//go:build !js

package spacewave_cli

import (
	"testing"

	"github.com/aperturerobotics/cli"
)

// TestBuildCanvasNodeAddSubcommands checks the canvas node add subcommands'
// names, arguments, and default size for each node type.
func TestBuildCanvasNodeAddSubcommands(t *testing.T) {
	// Require the add command name.
	cmd := buildCanvasNodeAddCommand()
	if cmd.Name != "add" {
		t.Fatalf("add command name = %q, want %q", cmd.Name, "add")
	}

	// Require each node-add subcommand name, arguments, and default size.
	want := []struct {
		name      string
		argsUsage string
		width     float64
		height    float64
	}{
		{"text", "<content>", 200, 100},
		{"object", "<object-key>", 400, 300},
		{"shape", "", 150, 150},
		{"drawing", "", 300, 300},
	}
	if len(cmd.Subcommands) != len(want) {
		t.Fatalf("add subcommand count = %d, want %d", len(cmd.Subcommands), len(want))
	}
	for i, w := range want {
		sub := cmd.Subcommands[i]
		if sub.Name != w.name {
			t.Errorf("sub[%d].Name = %q, want %q", i, sub.Name, w.name)
		}
		if sub.ArgsUsage != w.argsUsage {
			t.Errorf("sub[%d].ArgsUsage = %q, want %q", i, sub.ArgsUsage, w.argsUsage)
		}
		if got := flagFloat64Default(sub.Flags, "width"); got != w.width {
			t.Errorf("sub[%d](%s) width default = %v, want %v", i, w.name, got, w.width)
		}
		if got := flagFloat64Default(sub.Flags, "height"); got != w.height {
			t.Errorf("sub[%d](%s) height default = %v, want %v", i, w.name, got, w.height)
		}
	}
}

func flagFloat64Default(flags []cli.Flag, name string) float64 {
	for _, f := range flags {
		ff, ok := f.(*cli.Float64Flag)
		if !ok || ff.Name != name {
			continue
		}
		return ff.Value
	}
	return -1
}
