//go:build !js

package main

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/prototypes/sync-library/lean"
)

func TestRun(t *testing.T) {
	if err := run(context.Background()); err != nil {
		t.Fatal(err.Error())
	}
}

func TestRunLean(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20000000000)
	defer cancel()
	if err := lean.RunLean(ctx); err != nil {
		t.Fatal(err.Error())
	}
}
