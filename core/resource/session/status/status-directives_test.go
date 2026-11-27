package status

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	cdc "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/sirupsen/logrus"
)

// namedDirective is a directive identified by its name.
type namedDirective struct{ name string }

func (d *namedDirective) GetName() string                         { return d.name }
func (d *namedDirective) Validate() error                         { return nil }
func (d *namedDirective) GetValueOptions() directive.ValueOptions { return directive.ValueOptions{} }
func (d *namedDirective) GetDebugVals() directive.DebugValues     { return nil }

// childAddingController resolves the parent directive by adding the child.
type childAddingController struct {
	add func(ctx context.Context) error
}

func (c *childAddingController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/child-adder", controller.MustParseVersion("0.0.1"), "adds the child")
}

func (c *childAddingController) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	if di.GetDirective().GetName() != "parent" {
		return nil, nil
	}
	return []directive.Resolver{directive.NewFuncResolver(func(ctx context.Context, _ directive.ResolverHandler) error {
		return c.add(ctx)
	})}, nil
}

func (c *childAddingController) Execute(context.Context) error { return nil }
func (c *childAddingController) Close() error                  { return nil }

func TestBuildDirectiveInfosLinksOrigins(t *testing.T) {
	// Build a bus where resolving "parent" adds "child".
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	b := inmem.NewBus(cdc.NewController(ctx, le))
	ctrl := &childAddingController{add: func(ctx context.Context) error {
		// Hold the child until the resolver stops.
		_, ref, err := b.AddDirective(ctx, &namedDirective{name: "child"}, nil)
		if err != nil {
			return err
		}
		<-ctx.Done()
		ref.Release()
		return nil
	}}
	rel, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// Add the parent.
	_, ref, err := b.AddDirective(ctx, &namedDirective{name: "parent"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()

	// Wait for the child to appear, then check both entries.
	for len(b.GetDirectives()) != 2 {
		select {
		case <-ctx.Done():
			t.Fatal("child directive was not added")
		case <-time.After(5 * time.Millisecond):
		}
	}
	byName := make(map[string]uint32)
	infos := buildDirectiveInfos(b.GetDirectives())
	for _, info := range infos {
		byName[info.GetName()] = info.GetId()
	}
	for _, info := range infos {
		refs := info.GetReferences()
		if len(refs) != 1 {
			t.Fatalf("%s has %d references, want 1", info.GetName(), len(refs))
		}
		switch info.GetName() {
		case "parent":
			if refs[0].GetOriginDirectiveId() != 0 || refs[0].GetOriginControllerId() != "" {
				t.Fatalf("parent reference has origin %v", refs[0])
			}
		case "child":
			if refs[0].GetOriginDirectiveId() != byName["parent"] || refs[0].GetOriginDirectiveIdent() != "parent" {
				t.Fatalf("child origin directive = %d %q, want %d parent", refs[0].GetOriginDirectiveId(), refs[0].GetOriginDirectiveIdent(), byName["parent"])
			}
			if refs[0].GetOriginControllerId() != "test/child-adder" {
				t.Fatalf("child origin controller = %q", refs[0].GetOriginControllerId())
			}
		}
	}
}
