package dist_entrypoint

import (
	"context"
	"io/fs"

	bldr_dist "github.com/aperturerobotics/bldr/dist"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// PostStartHook is a post start function.
type PostStartHook func(distBus *DistBus) (rels []func(), err error)

// Run builds the bus & starts the dist entrypoint.
func Run(
	ctx context.Context,
	le *logrus.Entry,
	distMeta *bldr_dist.DistMeta,
	assetsFS fs.FS,
	webRuntimeID string,
	postStartHooks []PostStartHook,
) error {
	if err := distMeta.Validate(); err != nil {
		return errors.Wrap(err, "dist_meta")
	}

	// allow configuring the storage root via an environment variable.
	projectID := distMeta.GetProjectId()
	storageRoot, err := DetermineStorageRoot(projectID)
	if err != nil {
		le.WithError(err).Warn("unable to determine storage root, using current dir")
		storageRoot = "./" + projectID
	}

	// mount the config set
	configSetBinFilename := "config-set.bin"
	configSetData, err := fs.ReadFile(assetsFS, configSetBinFilename)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		configSetData = nil
	}

	configSetProto := &configset_proto.ConfigSet{}
	if err := configSetProto.UnmarshalVT(configSetData); err != nil {
		return err
	}

	verbose := false // TODO
	staticBlockStoreReaderBuilder := newStaticBlockStoreReaderBuilder(le, assetsFS, verbose)

	distBus, err := BuildDistBus(
		ctx,
		le,
		distMeta,
		storageRoot,
		webRuntimeID,
		configSetProto,
		staticBlockStoreReaderBuilder,
	)
	if err != nil {
		return errors.Wrap(err, "unable to initialize")
	}
	defer distBus.Release()

	// run any post-start hooks (starts web runtime on web platform)
	for _, hook := range postStartHooks {
		rels, err := hook(distBus)
		for _, rel := range rels {
			defer rel()
		}
		if err != nil {
			return err
		}
	}

	// wait for context to be canceled
	<-ctx.Done()
	return context.Canceled
}
