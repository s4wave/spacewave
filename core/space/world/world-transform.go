package space_world

import (
	"crypto/rand"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/space/world/cursorcodec"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/util/blockenc"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// NewInitWorldOp returns a copy of base, or an empty operation, with a fresh
// World transform when base specifies none. The author chooses the transform,
// so every member replays the same World.
func NewInitWorldOp(base *InitWorldOp) (*InitWorldOp, error) {
	// Copy the base.
	op := base.CloneVT()
	if op == nil {
		op = &InitWorldOp{}
	}

	// Choose a transform when the base specifies none.
	if op.GetTransformConf().GetEmpty() {
		conf, err := buildDefaultTransformConf()
		if err != nil {
			return nil, err
		}
		op.TransformConf = conf
	}
	return op, nil
}

// SpaceInitWorldOp returns the operation a new Space's World starts from: the
// newest format and no changelog, matching the default SpaceSettings. A Space
// seeded with only its World's transform writes this root at its first change,
// so every replay of a Space's operations, live or transient, builds the same
// World whatever its adapter was configured with.
func SpaceInitWorldOp() *InitWorldOp {
	return &InitWorldOp{LastChangeDisable: true, Format: world_block.NewestWorldFormat}
}

// Validate checks the operation's transform, format and graph settings.
func (i *InitWorldOp) Validate() error {
	if err := i.GetTransformConf().Validate(); err != nil {
		return err
	}
	return i.GetFormat().ValidateInitial(i.GetGraphSettings())
}

// BuildInitialInnerState builds the empty World initOp initializes. initOp
// must specify an authenticated World transform.
func BuildInitialInnerState(initOp *InitWorldOp) (*InnerState, error) {
	// Require an authenticated transform.
	transformConf := initOp.GetTransformConf()
	if transformConf.GetEmpty() {
		return nil, errors.New("world init operation names no transform")
	}
	if err := cursorcodec.ValidateTransform(transformConf); err != nil {
		return nil, err
	}

	// Start the World with no root.
	return &InnerState{HeadRef: &bucket.ObjectRef{TransformConf: transformConf}}, nil
}

// validateTransformSuccessor checks that next is a valid World write transform
// that reads every block prev wrote: the same steps in the same order, where
// each step is unchanged or a gzip step became an s2 step that reads gzip.
func validateTransformSuccessor(prev, next *block_transform.Config) error {
	// The new transform must be one the World may write with.
	if err := cursorcodec.ValidateTransform(next); err != nil {
		return err
	}

	// Compare the steps in order.
	prevSteps, nextSteps := prev.GetSteps(), next.GetSteps()
	if len(prevSteps) != len(nextSteps) {
		return errors.New("new world transform must keep the same steps")
	}
	for i, prevStep := range prevSteps {
		if err := validateStepSuccessor(prevStep, nextSteps[i]); err != nil {
			return errors.Wrapf(err, "world transform step %d", i)
		}
	}
	return nil
}

// validateStepSuccessor checks that next decodes every block prev encoded.
func validateStepSuccessor(prev, next *block_transform.StepConfig) error {
	// An unchanged step reads its own output.
	if prev.EqualVT(next) {
		return nil
	}

	// s2 with read_gzip reads gzip output.
	if prev.GetId() != transform_gzip.ConfigID || next.GetId() != transform_s2.ConfigID {
		return errors.Errorf("cannot replace %s with %s", prev.GetId(), next.GetId())
	}
	s2Conf := &transform_s2.Config{}
	if err := block_transform.UnmarshalStepConfig(next.GetConfig(), s2Conf); err != nil {
		return errors.Wrap(err, "s2 config")
	}
	if !s2Conf.GetReadGzip() {
		return errors.New("s2 step replacing gzip must set read_gzip")
	}
	return nil
}

// buildDefaultTransformConf builds a fresh World transform: block encryption
// with a random key.
func buildDefaultTransformConf() (*block_transform.Config, error) {
	// Derive a key from random material.
	var encKey [32]byte
	var material [64]byte
	if _, err := rand.Read(material[:]); err != nil {
		return nil, err
	}
	defer scrub.Scrub(material[:])

	// Derive the key under the label, which refers to
	// core/sobject/world/engine, the deleted package
	// buildDefaultTransformConf came from. The label separates this
	// derivation from others, not packages, so the label keeps the old
	// path.
	blockenc.DeriveKeySHA256("sobject/world/engine transform-config 2026-06-08 sha256 v1.", material[:], encKey[:])

	// Encrypt with it.
	return block_transform.NewConfig([]config.Config{&transform_blockenc.Config{
		BlockEnc: blockenc.DefaultBlockEnc,
		Key:      encKey[:],
	}})
}
