//go:build js

// Package main prototypes Spacewave as an embeddable sync library: it creates
// a Hydra world in-process, stores objects and a graph edge, and reads them
// back through the same WorldState API a TypeScript SDK would expose.
package main

import (
	"context"
	"os"

	"github.com/s4wave/spacewave/prototypes/sync-library/lean"
)

func main() {
	ctx := context.Background()
	var err error
	if len(os.Args) > 1 && os.Args[1] == "lean" {
		err = lean.RunLean(ctx)
	} else {
		err = run(ctx)
	}
	if err != nil {
		panic(err)
	}
}
