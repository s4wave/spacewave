//go:build !js

package bldr_project_validate

import (
	"context"
	"os"
	"testing"

	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
)

// TestMain runs the bounded evaluation child when the test binary is started
// as one, as the spacewave command does, and the tests otherwise.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == bldr_project_starlark.BoundedCommand {
		if err := bldr_project_starlark.RunBounded(context.Background(), os.Stdin, os.Stdout); err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
