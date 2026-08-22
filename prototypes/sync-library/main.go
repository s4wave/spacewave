//go:build js

// Package main prototypes Spacewave as an embeddable sync library: it creates
// a Hydra world in-process, stores objects and a graph edge, and reads them
// back through the same WorldState API a TypeScript SDK would expose.
package main

import "context"

func main() {
	if err := run(context.Background()); err != nil {
		panic(err)
	}
}
