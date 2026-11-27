package s4wave_vm

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/cayley/quad"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Graph predicates link a WasiVmPackage to its layers and dependencies, and
// a WasiVmImage to the packages it asks for.
var (
	// PredWasiVmPackageRuntime links a package to its runtime layer.
	PredWasiVmPackageRuntime = quad.IRI("wasivmpkg/runtime")
	// PredWasiVmPackageDev links a package to its dev layer.
	PredWasiVmPackageDev = quad.IRI("wasivmpkg/dev")
	// PredWasiVmPackageDepends links a package to each runtime dependency.
	PredWasiVmPackageDepends = quad.IRI("wasivmpkg/depends")
	// PredWasiVmImagePackage links an image to each package it asks for.
	PredWasiVmImagePackage = quad.IRI("wasivmimage/package")
)

// WasiVmPackageKey returns the object key of a package version, such as
// wasivm-pkg/app-editors/nano/9.2-r1. The package's layers live under the
// package key at runtime and dev.
func WasiVmPackageKey(name, version string) string {
	return "wasivm-pkg/" + name + "/" + version
}

// Validate performs cursory checks on the WasiVmPackage.
func (p *WasiVmPackage) Validate() error {
	if !strings.Contains(p.GetName(), "/") {
		return errors.Errorf("name must be category/name: %q", p.GetName())
	}
	if p.GetVersion() == "" {
		return errors.New("version is required")
	}
	return nil
}

// NewCreateWasiVmPackageOp constructs a new CreateWasiVmPackageOp.
func NewCreateWasiVmPackageOp(objKey string, pkg *WasiVmPackage, ts time.Time) *CreateWasiVmPackageOp {
	return &CreateWasiVmPackageOp{
		ObjectKey: objKey,
		Package:   pkg,
		Timestamp: timestamppb.New(ts),
	}
}

// Validate performs cursory checks on the op.
func (o *CreateWasiVmPackageOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if o.GetPackage() == nil {
		return errors.New("package is required")
	}
	return o.GetPackage().Validate()
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateWasiVmPackageOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Validate the package creation intent.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Create the package object with the operation's creation timestamp.
	objKey := o.GetObjectKey()
	pkg := o.GetPackage().CloneVT()
	pkg.CreatedAt = o.GetTimestamp()
	createdObject, _, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(pkg, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return false, err
	}

	// Mark the object with its block type.
	if err := world_types.SetObjectType(ctx, ws, objKey, WasiVmPackageTypeID); err != nil {
		return false, err
	}
	return false, nil
}

// LookupWasiVmImagePackages returns the keys of the packages the image at
// imageKey asks for, sorted.
func LookupWasiVmImagePackages(ctx context.Context, ws world.WorldStateGraph, imageKey string) ([]string, error) {
	return lookupEdgeTargets(ctx, ws, imageKey, string(PredWasiVmImagePackage))
}

// ResolveWasiVmPackages returns the runtime closure of the packages at keys,
// each package after the packages it depends on, which is the order their
// runtime layers are laid onto a root. Dependencies are visited in key order,
// so the same catalog always yields the same plan. A dependency cycle is
// broken where it is found.
func ResolveWasiVmPackages(ctx context.Context, ws world.WorldStateGraph, keys []string) ([]string, error) {
	// Visit each package depth first and emit it after its dependencies. A
	// package is marked 1 while its dependencies are visited, so a cycle ends
	// there, and 2 once emitted.
	var plan []string
	state := make(map[string]int)
	var visit func(key string) error
	visit = func(key string) error {
		// Skip a package already visited or on the current path.
		if state[key] != 0 {
			return nil
		}

		// Visit the dependencies, then emit the package.
		state[key] = 1
		deps, err := lookupEdgeTargets(ctx, ws, key, string(PredWasiVmPackageDepends))
		if err != nil {
			return err
		}
		for _, dep := range deps {
			if err := visit(dep); err != nil {
				return errors.Wrap(err, key)
			}
		}
		state[key] = 2
		plan = append(plan, key)
		return nil
	}

	// Resolve every requested package.
	for _, key := range keys {
		if err := visit(key); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

// lookupEdgeTargets returns the sorted object keys targeted by every edge
// from subject with the predicate.
func lookupEdgeTargets(ctx context.Context, ws world.WorldStateGraph, subject, pred string) ([]string, error) {
	// Read the edges and parse their targets.
	gqs, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(subject, pred, "", ""), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "look up %s edges of %s", pred, subject)
	}
	keys := make([]string, 0, len(gqs))
	for _, gq := range gqs {
		key, err := world.GraphValueToKey(gq.GetObj())
		if err != nil {
			return nil, errors.Wrapf(err, "parse %s edge of %s", pred, subject)
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys, nil
}

// _ is a type assertion
var (
	_ block.Block     = (*WasiVmPackage)(nil)
	_ world.Operation = (*CreateWasiVmPackageOp)(nil)
)
