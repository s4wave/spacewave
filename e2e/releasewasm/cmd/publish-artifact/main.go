//go:build !js

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aperturerobotics/util/gitroot"
	"github.com/s4wave/spacewave/e2e/releasewasm"
)

func main() {
	repoRoot, err := gitroot.FindRepoRoot()
	if err == nil {
		err = releasewasm.PublishReleaseWasmArtifact(context.Background(), repoRoot)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
