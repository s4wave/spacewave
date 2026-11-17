//go:build !js && !wasip1

package world_recovery

import (
	"context"
	"errors"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

// openWorld reads a native immutable WorldState without Engine transaction wrappers.
func (s *Scanner) openWorld(ctx context.Context, index uint32) (*world_block.WorldState, func(), error) {
	// Admit bounded inline transforms before constructing any native reader.
	ref := s.source.GetWorlds()[index].GetRef()
	if err := validateContext(ref); err != nil {
		return nil, nil, err
	}
	logger := logrus.NewEntry(logrus.New())
	factories := transform_all.BuildFactorySet()
	transformer, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: logger}, factories, ref.GetTransformConf())
	if err != nil {
		return nil, nil, err
	}

	// Every native capability resolves to copied scratch bytes, never the source.
	cursor := bucket_lookup.NewCursor(ctx, nil, logger, factories, s.blocks, transformer, ref.CloneVT(), &bucket.BucketOpArgs{BucketId: ref.GetBucketId()}, ref.GetTransformConf())
	state, err := world_block.BuildWorldStateFromCursor(ctx, logger, false, cursor, nil, nil, false)
	if err != nil {
		cursor.Release()
		return nil, nil, err
	}
	return state, func() { state.Discard(); cursor.Release() }, nil
}

// readObjectType reads at most two native type quads, preserving ambiguity as partial evidence.
func readObjectType(ctx context.Context, state world.WorldState, key string) (string, error) {
	// A bounded exact graph lookup avoids collecting the general metadata API's million-row limit.
	quads, err := state.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, world_types.TypePred.String(), "", ""), 2)
	if err != nil {
		return "", err
	}
	if len(quads) == 0 {
		return "", nil
	}
	if len(quads) != 1 {
		return "", errors.New("object has ambiguous historical type edges")
	}

	// Interpret only the native type-object key grammar.
	typeKey, err := world.GraphValueToKey(quads[0].GetObj())
	if err != nil {
		return "", err
	}
	id, ok := strings.CutPrefix(typeKey, world_types.TypesPrefix)
	if !ok {
		return "", errors.New("object type edge has an unsupported endpoint")
	}
	return id, nil
}
