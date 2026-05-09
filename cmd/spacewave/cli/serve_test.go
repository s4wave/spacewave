//go:build !js

package spacewave_cli

import (
	"flag"
	"io"
	"testing"
)

func TestServeCommandTraceFlag(t *testing.T) {
	cmd := newServeCommand(nil)
	set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	for _, fl := range cmd.Flags {
		if err := fl.Apply(set); err != nil {
			t.Fatalf("apply flag: %v", err)
		}
	}
	if set.Lookup("trace") == nil {
		t.Fatal("trace flag missing")
	}
}
