package s4wave_apt

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CreateAptRepositoryOpId is the operation id for CreateAptRepositoryOp.
var CreateAptRepositoryOpId = "spacewave-vm/apt/repository/create"

// AddAptPackageOpId is the operation id for AddAptPackageOp.
var AddAptPackageOpId = "spacewave-vm/apt/package/add"

// AddAptBuildSpecOpId is the operation id for AddAptBuildSpecOp.
var AddAptBuildSpecOpId = "spacewave-vm/apt/build-spec/add"

// NewCreateAptRepositoryOp constructs a new CreateAptRepositoryOp.
func NewCreateAptRepositoryOp(objectKey string, repository *AptRepository) *CreateAptRepositoryOp {
	return &CreateAptRepositoryOp{
		ObjectKey:  objectKey,
		Repository: repository,
	}
}

// NewCreateAptRepositoryOpBlock constructs a new CreateAptRepositoryOp block.
func NewCreateAptRepositoryOpBlock() block.Block {
	return &CreateAptRepositoryOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *CreateAptRepositoryOp) GetOperationTypeId() string {
	return CreateAptRepositoryOpId
}

// Validate performs cursory checks on the op.
func (o *CreateAptRepositoryOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if o.GetRepository() == nil {
		return errors.New("repository is required")
	}
	return o.GetRepository().Validate()
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateAptRepositoryOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	objectKey := o.GetObjectKey()
	repository := o.GetRepository().CloneVT()
	_, _, err = world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(repository, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	if err := world_types.SetObjectType(ctx, ws, objectKey, AptRepositoryTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CreateAptRepositoryOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CreateAptRepositoryOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (o *CreateAptRepositoryOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCreateAptRepositoryOp looks up a CreateAptRepositoryOp operation type.
func LookupCreateAptRepositoryOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CreateAptRepositoryOpId {
		return &CreateAptRepositoryOp{}, nil
	}
	return nil, nil
}

// NewAddAptPackageOp constructs a new AddAptPackageOp.
func NewAddAptPackageOp(repositoryKey, packageKey string, aptPackage *AptPackage) *AddAptPackageOp {
	return &AddAptPackageOp{
		RepositoryKey: repositoryKey,
		PackageKey:    packageKey,
		AptPackage:    aptPackage,
	}
}

// NewAddAptPackageOpBlock constructs a new AddAptPackageOp block.
func NewAddAptPackageOpBlock() block.Block {
	return &AddAptPackageOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *AddAptPackageOp) GetOperationTypeId() string {
	return AddAptPackageOpId
}

// Validate performs cursory checks on the op.
func (o *AddAptPackageOp) Validate() error {
	if o.GetRepositoryKey() == "" {
		return errors.New("repository_key is required")
	}
	if o.GetPackageKey() == "" {
		return errors.New("package_key is required")
	}
	if o.GetAptPackage() == nil {
		return errors.New("apt_package is required")
	}
	return o.GetAptPackage().Validate()
}

// ApplyWorldOp applies the operation as a world operation.
func (o *AddAptPackageOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	if err := world_types.CheckObjectType(ctx, ws, o.GetRepositoryKey(), AptRepositoryTypeID); err != nil {
		return false, err
	}

	packageKey := o.GetPackageKey()
	aptPackage := o.GetAptPackage().CloneVT()
	_, _, err = world.CreateWorldObject(ctx, ws, packageKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(aptPackage, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	if err := world_types.SetObjectType(ctx, ws, packageKey, AptPackageTypeID); err != nil {
		return false, err
	}
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
		o.GetRepositoryKey(),
		PredAptRepoPackage.String(),
		packageKey,
		"",
	)); err != nil {
		return true, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *AddAptPackageOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *AddAptPackageOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (o *AddAptPackageOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupAddAptPackageOp looks up an AddAptPackageOp operation type.
func LookupAddAptPackageOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == AddAptPackageOpId {
		return &AddAptPackageOp{}, nil
	}
	return nil, nil
}

// NewAddAptBuildSpecOp constructs a new AddAptBuildSpecOp.
func NewAddAptBuildSpecOp(repositoryKey, buildSpecKey string, buildSpec *AptBuildSpec) *AddAptBuildSpecOp {
	return &AddAptBuildSpecOp{
		RepositoryKey: repositoryKey,
		BuildSpecKey:  buildSpecKey,
		BuildSpec:     buildSpec,
	}
}

// NewAddAptBuildSpecOpBlock constructs a new AddAptBuildSpecOp block.
func NewAddAptBuildSpecOpBlock() block.Block {
	return &AddAptBuildSpecOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *AddAptBuildSpecOp) GetOperationTypeId() string {
	return AddAptBuildSpecOpId
}

// Validate performs cursory checks on the op.
func (o *AddAptBuildSpecOp) Validate() error {
	if o.GetRepositoryKey() == "" {
		return errors.New("repository_key is required")
	}
	if o.GetBuildSpecKey() == "" {
		return errors.New("build_spec_key is required")
	}
	if o.GetBuildSpec() == nil {
		return errors.New("build_spec is required")
	}
	return o.GetBuildSpec().Validate()
}

// ApplyWorldOp applies the operation as a world operation.
func (o *AddAptBuildSpecOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	if err := world_types.CheckObjectType(ctx, ws, o.GetRepositoryKey(), AptRepositoryTypeID); err != nil {
		return false, err
	}

	buildSpecKey := o.GetBuildSpecKey()
	buildSpec := o.GetBuildSpec().CloneVT()
	_, _, err = world.CreateWorldObject(ctx, ws, buildSpecKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(buildSpec, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	if err := world_types.SetObjectType(ctx, ws, buildSpecKey, AptBuildSpecTypeID); err != nil {
		return false, err
	}
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
		o.GetRepositoryKey(),
		PredAptRepoBuildSpec.String(),
		buildSpecKey,
		"",
	)); err != nil {
		return true, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *AddAptBuildSpecOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *AddAptBuildSpecOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (o *AddAptBuildSpecOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupAddAptBuildSpecOp looks up an AddAptBuildSpecOp operation type.
func LookupAddAptBuildSpecOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == AddAptBuildSpecOpId {
		return &AddAptBuildSpecOp{}, nil
	}
	return nil, nil
}

// LookupAptOp looks up built-in Apt operation types.
func LookupAptOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		LookupCreateAptRepositoryOp,
		LookupAddAptPackageOp,
		LookupAddAptBuildSpecOp,
	}).LookupOp(ctx, operationTypeID)
}

// _ is a type assertion
var (
	_ world.Operation = ((*CreateAptRepositoryOp)(nil))
	_ world.Operation = ((*AddAptPackageOp)(nil))
	_ world.Operation = ((*AddAptBuildSpecOp)(nil))
)
