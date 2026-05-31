//go:build !skip_e2e && !js

package wasm

import (
	"errors"
	"testing"
)

func TestTransientAppWaitErrorIncludesStartupNavigation(t *testing.T) {
	err := errors.New("playwright: Execution context was destroyed, most likely because of a navigation")
	if !isTransientAppWaitError(err) {
		t.Fatalf("startup navigation error should be retried")
	}
}
