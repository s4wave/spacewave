//go:build !js

package main

import (
	"context"
	"testing"
)

func TestRun(t *testing.T) {
	if err := run(context.Background()); err != nil {
		t.Fatal(err.Error())
	}
}
