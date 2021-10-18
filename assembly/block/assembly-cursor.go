package assembly_block

import (
	"context"

	"github.com/aperturerobotics/bldr/assembly"
	"github.com/aperturerobotics/controllerbus/bus"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/hydra/block"
)

// AssemblyCursor is a Assembly with an attached Block cursor.
type AssemblyCursor struct {
	// a is the assembly object.
	a *Assembly
	// bcs is the block cursor at sa
	bcs *block.Cursor
}

// NewAssemblyCursor builds a new AssemblyCursor.
func NewAssemblyCursor(a *Assembly, bcs *block.Cursor) *AssemblyCursor {
	return &AssemblyCursor{a: a, bcs: bcs}
}

// ResolveControllerExec resolves the controller exec configuration for the Assembly.
// return nil if no controller exec configured.
func (r *AssemblyCursor) ResolveControllerExec(ctx context.Context, b bus.Bus) (*controller_exec.ExecControllerRequest, error) {
	if r == nil {
		return nil, nil
	}
	return r.a.GetControllerExec(), nil
}

// ResolveSubAssemblies resolves the list of sub assembly bus to run.
// Can be configured to optionally inherit parent plugins and resolvers.
func (r *AssemblyCursor) ResolveSubAssemblies(ctx context.Context, b bus.Bus) ([]assembly.SubAssembly, error) {
	if r == nil {
		return nil, nil
	}
	s := r.a.GetSubAssemblies()
	sa := make([]assembly.SubAssembly, len(s))
	subAssemblySet := NewSubAssemblySet(&r.a.SubAssemblies, r.bcs.FollowSubBlock(2))
	for i := range s {
		_, siBcs := subAssemblySet.Get(i)
		sa[i] = NewSubAssemblyCursor(s[i], siBcs)
	}
	return sa, nil
}

// _ is a type assertion
var _ assembly.Assembly = ((*AssemblyCursor)(nil))
