package web_view_handler_controller

import (
	"context"
	"testing"
)

func TestControllerExecuteCanReturnNilWhenIdle(t *testing.T) {
	ctrl := NewController(nil, nil)
	if err := ctrl.Execute(context.Background()); err != nil {
		t.Fatalf("Execute returned %v, want nil", err)
	}
}
