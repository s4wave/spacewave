package mysql_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/object"
	hydra_sql "github.com/aperturerobotics/hydra/sql"
	sql_mysql "github.com/aperturerobotics/hydra/sql/mysql"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the controller.
const ControllerID = "hydra/sql/mysql"

// Version is the API version.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "access object store backed sql db"

// Controller implements the MySQL controller.
type Controller struct {
	*hydra_sql.Controller
	le   *logrus.Entry
	b    bus.Bus
	conf *Config

	sfs       *block_transform.StepFactorySet
	stateXfrm *block_transform.Transformer
}

// NewController constructs a new MySQL controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config, sfs *block_transform.StepFactorySet) (*Controller, error) {
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le},
		sfs,
		conf.GetStateTransformConf(),
	)
	if err != nil {
		return nil, err
	}
	ctrl := &Controller{
		le:   le,
		b:    b,
		conf: conf,

		sfs:       sfs,
		stateXfrm: xfrm,
	}
	ctrl.Controller = hydra_sql.NewController(
		controller.NewInfo(ControllerID, Version, controllerDescrip),
		conf.GetSqlDbId(),
		ctrl.executeDB,
	)
	return ctrl, nil
}

// executeDB executes the mysql setup logic.
func (c *Controller) executeDB(ctx context.Context, ctr *ccontainer.CContainer[*hydra_sql.SqlStore]) error {
	le := c.le

	rctx, rctxCancel := context.WithCancel(ctx)
	defer rctxCancel()

	// Determine the init ref to the HEAD
	var headRef *bucket.ObjectRef

	// initialize headRef using the configured head ref
	initRef := c.conf.GetInitHeadRef()
	if initRef != nil {
		headRef = initRef.Clone()
	}

	// Lookup the state store
	stateStoreID := c.conf.GetObjectStoreId()
	stateStoreVol := c.conf.GetVolumeId()
	if stateStoreVol == "" {
		le.Debug("no volume id set, using any available volume")
	}

	var stateStore object.ObjectStore
	if stateStoreID != "" {
		storeVal, _, storeRef, err := volume.BuildObjectStoreAPIEx(ctx, c.b, false, stateStoreID, stateStoreVol, nil)
		if err != nil {
			return err
		}
		defer storeRef.Release()
		if err := storeVal.GetError(); err != nil {
			return err
		}
		stateStore = storeVal.GetObjectStore()
	}
	var headState *HeadState
	if stateStore != nil {
		// apply object store prefix
		if prefix := c.conf.GetObjectStorePrefix(); len(prefix) != 0 {
			stateStore = object.NewPrefixer(stateStore, []byte(prefix))
		}
		// load initial head ref
		var headStateFound bool
		var err error
		headState, headStateFound, err = c.loadHeadState(ctx, stateStore)
		if err != nil {
			return err
		}
		if headStateFound {
			headRef = headState.GetHeadRef()
		}
	} else {
		le.Debug("state store is not configured, changes will not be persisted")
		if headRef.GetEmpty() {
			le.Debug("no initial head reference provided, initializing empty db")
		}
	}
	if headRef == nil {
		headRef = &bucket.ObjectRef{}
	}
	// override bucket id if configured
	if confBucketID := c.conf.GetBucketId(); confBucketID != "" {
		headRef.BucketId = confBucketID
	}
	if headRef.GetBucketId() == "" {
		return errors.New("head ref bucket id required but was unset")
	}

	le.Debug("building sql database")
	cursor, err := bucket_lookup.BuildCursor(
		ctx,
		c.b,
		le,
		c.sfs,
		c.conf.GetVolumeId(),
		headRef,
		nil,
	)
	if err != nil {
		return err
	}
	defer cursor.Release()

	var commitFn sql_mysql.CommitFn
	if stateStore != nil {
		commitFn = func(nref *bucket.ObjectRef) error {
			// write state back to state store
			return c.writeHeadState(ctx, stateStore, nref)
		}
	}

	mysql := sql_mysql.NewMysql(cursor, commitFn)
	createDBs := c.conf.GetCreateDbs()
	if len(createDBs) != 0 {
		tx, err := mysql.NewMysqlTransaction(true)
		if err != nil {
			return err
		}
		for _, dbName := range c.conf.GetCreateDbs() {
			_, err := tx.OpenDatabase(dbName, true)
			if err != nil {
				tx.Discard()
				return err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return errors.Wrap(err, "commit create dbs")
		}
	}

	le.Info("sql store ready")
	var handle hydra_sql.SqlStore = mysql
	ctr.SetValue(&handle)
	<-rctx.Done()

	le.Debug("shutting down")
	ctr.SetValue(nil)
	return context.Canceled
}
