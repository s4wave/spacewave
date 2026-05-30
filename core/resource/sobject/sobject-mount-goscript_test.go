//go:build goscript

package resource_sobject

import (
	"context"
	"testing"

	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
)

const testMountSharedObjectBodyAvailable = false

func TestMountSharedObjectBodyReturnsTypedHealthResponseWhenGoScriptUnavailable(t *testing.T) {
	resourceCtx := &testResourceClientContext{}
	ctx := resource_server.WithResourceClientContext(context.Background(), resourceCtx)
	r := &SharedObjectResource{
		meta: &sobject.SharedObjectMeta{BodyType: "space"},
	}

	resp, err := r.MountSharedObjectBody(
		ctx,
		&s4wave_sobject.MountSharedObjectBodyRequest{},
	)
	if err != nil {
		t.Fatalf("MountSharedObjectBody() error = %v", err)
	}
	if resp.GetResourceId() != 0 {
		t.Fatalf("expected no body resource id, got %d", resp.GetResourceId())
	}

	health := resp.GetHealth()
	if health == nil {
		t.Fatal("expected typed health response")
	}
	if health.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_CLOSED {
		t.Fatalf("expected closed status, got %v", health.GetStatus())
	}
	if health.GetLayer() != sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY {
		t.Fatalf("expected body layer, got %v", health.GetLayer())
	}
	if health.GetError() != errMountSharedObjectBodyUnavailable.Error() {
		t.Fatalf("expected unavailable detail, got %q", health.GetError())
	}
	if resourceCtx.nextID != 0 {
		t.Fatalf("expected no child resources to be registered, got %d", resourceCtx.nextID)
	}
}
