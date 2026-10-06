//go:build !js

package main

import (
	"io"
	"os"

	"github.com/pkg/errors"
)

func main() {
	if err := run(); err != nil {
		_, _ = io.WriteString(os.Stderr, err.Error()+"\n")
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: plugin-release pack-world|write-handoff-manifest [flags]")
	}

	switch os.Args[1] {
	case "pack-world":
		return runPackWorld(os.Args[2:])
	case "write-handoff-manifest":
		return runWritePluginHandoffManifest(os.Args[2:])
	default:
		return errors.Errorf("unknown command %q", os.Args[1])
	}
}
