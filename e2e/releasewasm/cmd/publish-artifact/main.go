//go:build !js

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aperturerobotics/util/gitroot"
	"github.com/s4wave/spacewave/e2e/releasewasm"
	"github.com/sirupsen/logrus"
)

func main() {
	le := logrus.NewEntry(logrus.New())
	repoRoot, err := gitroot.FindRepoRoot()
	if err == nil {
		err = releasewasm.PublishReleaseWasmArtifact(context.Background(), le, repoRoot)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
