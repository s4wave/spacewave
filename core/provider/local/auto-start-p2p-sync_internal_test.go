package provider_local

import (
	"context"
	"testing"

	"github.com/pkg/errors"
)

func TestIsAutoStartReadCanceled(t *testing.T) {
	if !isAutoStartReadCanceled(errors.Wrap(context.Canceled, "mount account settings")) {
		t.Fatal("expected wrapped context cancellation to be treated as canceled")
	}
	if isAutoStartReadCanceled(errors.New("mount account settings")) {
		t.Fatal("expected non-cancellation errors to remain warnings")
	}
}
