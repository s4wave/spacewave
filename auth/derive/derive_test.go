package auth_derive

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/identity"
)

func TestControllerDisablePromptPasswordLifecycle(t *testing.T) {
	dir := identity.NewDeriveEntityKeypair([]*identity.EntityKeypair{{}})
	di := &testDirectiveInstance{dir: dir}

	disabled, err := NewController(nil, nil, &Config{DisablePromptPassword: true})
	if err != nil {
		t.Fatal(err)
	}
	resolvers, err := disabled.HandleDirective(context.Background(), di)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolvers) != 0 {
		t.Fatalf("disabled controller returned %d prompt resolver(s)", len(resolvers))
	}

	compatible, err := NewController(nil, nil, &Config{})
	if err != nil {
		t.Fatal(err)
	}
	resolvers, err = compatible.HandleDirective(context.Background(), di)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolvers) != 1 {
		t.Fatalf("default controller returned %d resolvers, want 1", len(resolvers))
	}
}

func TestConfigEqualsIncludesDisablePromptPassword(t *testing.T) {
	if (&Config{}).EqualsConfig(&Config{DisablePromptPassword: true}) {
		t.Fatal("prompt policy change was treated as the same controller lifecycle config")
	}
	if !(&Config{}).EqualsConfig(&Config{}) {
		t.Fatal("identical prompt policy was treated as different")
	}
}

type testDirectiveInstance struct {
	dir directive.Directive
}

func (t *testDirectiveInstance) GetContext() context.Context       { return context.Background() }
func (t *testDirectiveInstance) GetDirective() directive.Directive { return t.dir }
func (t *testDirectiveInstance) GetDirectiveIdent() string         { return t.dir.GetName() }
func (t *testDirectiveInstance) GetResolverErrors() []error        { return nil }
func (t *testDirectiveInstance) AddReference(directive.ReferenceHandler, bool) directive.Reference {
	return nil
}

func (t *testDirectiveInstance) AddDisposeCallback(func()) func() { return func() {} }

func (t *testDirectiveInstance) AddIdleCallback(directive.IdleCallback) func() { return func() {} }

func (t *testDirectiveInstance) AddStateCallback(directive.StateCallback) func() { return func() {} }
func (t *testDirectiveInstance) CloseIfUnreferenced(bool) bool                   { return false }
func (t *testDirectiveInstance) Close()                                          {}
