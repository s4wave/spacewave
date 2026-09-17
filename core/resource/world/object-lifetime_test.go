//go:build !js

package resource_world_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
)

// TestObjectResourceRetiresUnderlyingHandle checks the ownership boundary at
// registration, including partial acquisition and rejected registration.
func TestObjectResourceRetiresUnderlyingHandle(t *testing.T) {
	failure := errors.New("injected failure")
	for name, invoke := range map[string]func(context.Context, *resource_world.WorldStateResource) error{
		"create": func(ctx context.Context, r *resource_world.WorldStateResource) error {
			_, err := r.CreateObject(ctx, &sdk_world.CreateObjectRequest{ObjectKey: "key"})
			return err
		},
		"get": func(ctx context.Context, r *resource_world.WorldStateResource) error {
			_, err := r.GetObject(ctx, &sdk_world.GetObjectRequest{ObjectKey: "key"})
			return err
		},
		"rename": func(ctx context.Context, r *resource_world.WorldStateResource) error {
			_, err := r.RenameObject(ctx, &sdk_world.RenameObjectRequest{OldObjectKey: "old", NewObjectKey: "key"})
			return err
		},
	} {
		for _, stage := range []string{"retirement", "acquisition error", "registration error"} {
			t.Run(name+"/"+stage, func(t *testing.T) {
				ctx := context.Background()
				obj := &rpcLifetimeObject{}
				ws := &rpcLifetimeWorld{obj: obj}
				resources := &rpcLifetimeContext{worldStateOperationResourceContext: &worldStateOperationResourceContext{ctx: ctx}}
				switch stage {
				case "acquisition error":
					ws.err = failure
				case "registration error":
					resources.err = failure
				}
				ctx = resource_server.WithResourceClientContext(ctx, resources)
				r := resource_world.NewWorldStateResource(nil, nil, ws, nil)
				err := invoke(ctx, r)
				if stage == "retirement" {
					if err != nil {
						t.Fatal(err)
					}
					if obj.releases != 0 {
						t.Fatal("object released while registered")
					}
					if !resources.ReleaseResource(1) {
						t.Fatal("missing registered resource")
					}
				} else if !errors.Is(err, failure) {
					t.Fatalf("error = %v, want injected failure", err)
				}
				if obj.releases != 1 {
					t.Fatalf("underlying releases = %d, want 1", obj.releases)
				}
			})
		}
	}
}

type rpcLifetimeObject struct {
	world.ObjectState
	releases int
}

func (o *rpcLifetimeObject) GetKey() string { return "key" }
func (o *rpcLifetimeObject) Release()       { o.releases++ }

type rpcLifetimeWorld struct {
	world.WorldState
	obj *rpcLifetimeObject
	err error
}

func (w *rpcLifetimeWorld) CreateObject(context.Context, string, *bucket.ObjectRef) (world.ObjectState, error) {
	return w.obj, w.err
}

func (w *rpcLifetimeWorld) GetObject(context.Context, string) (world.ObjectState, bool, error) {
	return w.obj, true, w.err
}

func (w *rpcLifetimeWorld) RenameObject(context.Context, string, string, bool) (world.ObjectState, error) {
	return w.obj, w.err
}

type rpcLifetimeContext struct {
	*worldStateOperationResourceContext
	err error
}

func (c *rpcLifetimeContext) AddResource(mux srpc.Invoker, release func()) (uint32, error) {
	if c.err != nil {
		return 0, c.err
	}
	return c.worldStateOperationResourceContext.AddResource(mux, release)
}
