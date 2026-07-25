package dist_entrypoint

import (
	"context"
	"io/fs"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	entrypoint_fatal "github.com/s4wave/spacewave/bldr/entrypoint/fatal"
	"github.com/s4wave/spacewave/db/block"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/sirupsen/logrus"
)

// DistBusHook is a function run against the dist bus during startup.
type DistBusHook func(distBus *DistBus) (rels []func(), err error)

// Run builds the bus & starts the dist entrypoint.
func Run(
	ctx context.Context,
	le *logrus.Entry,
	distMeta *bldr_dist.DistMeta,
	assetsFS fs.FS,
	webRuntimeID string,
	preBuildHooks []DistBusHook,
	postStartHooks []DistBusHook,
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
	staticBlockStoreReaderBuilder := newStaticBlockStoreReaderBuilder(
		le,
		assetsFS,
		verbose,
		distMeta.GetDistWorldRef().GetRootRef(),
	)

	distBus, err := BuildDistBus(
		ctx,
		le,
		distMeta,
		storageRoot,
		webRuntimeID,
		configSetProto,
		staticBlockStoreReaderBuilder,
		preBuildHooks,
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

	// wait for context cancellation or a process-fatal condition
	select {
	case <-ctx.Done():
		return context.Canceled
	case <-entrypoint_fatal.Chan():
		// A controller reported a condition under which the daemon
		// cannot serve, such as another live daemon owning the
		// front-door socket. Exit with the error instead of running
		// without the reporting controller.
		return entrypoint_fatal.Err()
	}
}

func validateStaticBlockStoreRoot(rdr *kvfile.Reader, rootRef *block.BlockRef) error {
	if rootRef == nil || rootRef.GetEmpty() {
		return nil
	}
	rootKey, err := rootRef.MarshalKey()
	if err != nil {
		return err
	}
	found, err := rdr.Exists(store_kvkey.NewDefaultKVKey().GetBlockKey(rootKey))
	if err != nil {
		return err
	}
	if !found {
		return errors.Wrap(block.ErrNotFound, rootRef.MarshalString())
	}
	return nil
}
