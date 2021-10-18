package assembly_block

import (
	"context"

	"github.com/aperturerobotics/bldr/assembly"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

// SubAssemblyCursor is a SubAssembly with an attached Block cursor.
type SubAssemblyCursor struct {
	// sa is the sub-assembly object.
	sa *SubAssembly
	// bcs is the block cursor at sa
	bcs *block.Cursor
}

// NewSubAssemblyCursor builds a new SubAssemblyCursor.
func NewSubAssemblyCursor(sa *SubAssembly, bcs *block.Cursor) *SubAssemblyCursor {
	return &SubAssemblyCursor{sa: sa, bcs: bcs}
}

// GetId returns the subassembly ID, used for logging and identification.
// Can be empty.
func (r *SubAssemblyCursor) GetId() string {
	return r.sa.GetId()
}

// ResolveAssemblies resolves the list of assembly to run on the SubAssembly bus.
func (r *SubAssemblyCursor) ResolveAssemblies(ctx context.Context, b bus.Bus) ([]assembly.Assembly, error) {
	// base list
	sa := r.sa
	a := sa.GetAssemblies()
	aSlice := NewAssemblySet(&sa.Assemblies, r.bcs.FollowSubBlock(1))
	arefs := sa.GetAssemblyRefs()
	aset := make([]assembly.Assembly, len(a), len(a)+len(arefs))
	for i := range a {
		ref := a[i]
		_, refBcs := aSlice.Get(i)
		aset[i] = NewAssemblyCursor(ref, refBcs)
	}

	// refs list
	arefsBcs := r.bcs.FollowSubBlock(2)
	for i, ref := range arefs {
		if ref.GetEmpty() {
			continue
		}
		aRefBcs := arefsBcs.FollowRef(uint32(i), ref)
		a, err := UnmarshalAssembly(aRefBcs)
		if err != nil {
			return nil, errors.Wrapf(err, "assembly_refs[%d]", i)
		}
		aset = append(aset, NewAssemblyCursor(a, aRefBcs))
	}

	return aset, nil
}

// ResolveDirectiveBridges resolves the list of directive bridges to apply.
func (r *SubAssemblyCursor) ResolveDirectiveBridges(ctx context.Context, b bus.Bus) ([]assembly.DirectiveBridge, error) {
	sa := r.sa
	a := sa.GetDirectiveBridges()
	aset := make([]assembly.DirectiveBridge, len(a))
	for i, db := range a {
		var err error
		aset[i], err = ResolveDirectiveBridgeCursor(ctx, b, db)
		if err != nil {
			return nil, err
		}
	}
	return aset, nil
}

// _ is a type assertion
var _ assembly.SubAssembly = ((*SubAssemblyCursor)(nil))
