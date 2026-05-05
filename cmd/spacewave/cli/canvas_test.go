//go:build !js

package spacewave_cli

import (
	"testing"

	"github.com/aperturerobotics/cli"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
)

// TestBuildCanvasNodeAddSubcommands pins the observable shape of the four
// canvas node add subcommands so the parameterized refactor preserves names,
// usage text, and default sizing per node type.
func TestBuildCanvasNodeAddSubcommands(t *testing.T) {
	cmd := buildCanvasNodeAddCommand()
	if cmd.Name != "add" {
		t.Fatalf("add command name = %q, want %q", cmd.Name, "add")
	}

	want := []struct {
		name      string
		usage     string
		argsUsage string
		width     float64
		height    float64
		nodeType  s4wave_canvas.NodeType
	}{
		{"text", "add a text node", "<content>", 200, 100, s4wave_canvas.NodeType_NODE_TYPE_TEXT},
		{"object", "add a world-object node", "<object-key>", 400, 300, s4wave_canvas.NodeType_NODE_TYPE_WORLD_OBJECT},
		{"shape", "add a shape node", "", 150, 150, s4wave_canvas.NodeType_NODE_TYPE_SHAPE},
		{"drawing", "add a drawing node", "", 300, 300, s4wave_canvas.NodeType_NODE_TYPE_DRAWING},
	}
	if len(cmd.Subcommands) != len(want) {
		t.Fatalf("add subcommand count = %d, want %d", len(cmd.Subcommands), len(want))
	}
	for i, w := range want {
		sub := cmd.Subcommands[i]
		if sub.Name != w.name {
			t.Errorf("sub[%d].Name = %q, want %q", i, sub.Name, w.name)
		}
		if sub.Usage != w.usage {
			t.Errorf("sub[%d].Usage = %q, want %q", i, sub.Usage, w.usage)
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
