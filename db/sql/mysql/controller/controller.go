package mysql_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/object"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	sql_rpc_server "github.com/s4wave/spacewave/db/sql/rpc/server"
	"github.com/s4wave/spacewave/db/volume"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the controller.
const ControllerID = "hydra/sql/mysql"

// Version is the API version.
var Version = controller.MustParseVersion("0.0.1")

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

	// Select the configured initial head reference.
	var headRef *bucket.ObjectRef

	// Resolve the configured initial head reference.
	initRef := c.conf.GetInitHeadRef()
	if initRef != nil {
		headRef = initRef.Clone()
	}

	// Resolve the object store used for persisted SQL state.
	stateStoreID := c.conf.GetObjectStoreId()
	stateStoreVol := c.conf.GetVolumeId()
	if stateStoreVol == "" {
		le.Debug("no volume id set, using any available volume")
	}

	var stateStore object.ObjectStore
	if stateStoreID != "" {
		storeVal, _, storeRef, err := volume.ExBuildObjectStoreAPI(ctx, c.b, false, stateStoreID, stateStoreVol, nil)
		if err != nil {
			return err
		}
		defer storeRef.Release()

		stateStore = storeVal.GetObjectStore()
	}
	var headState *HeadState
	if stateStore != nil {
		// Apply the configured object-store key prefix.
		if prefix := c.conf.GetObjectStorePrefix(); len(prefix) != 0 {
			stateStore = object.NewPrefixer(stateStore, []byte(prefix))
		}

		// Load the persisted head state when available.
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

	// Override the configured bucket and validate the selected head.
	if confBucketID := c.conf.GetBucketId(); confBucketID != "" {
		headRef.BucketId = confBucketID
	}
	if headRef.GetBucketId() == "" {
		return errors.New("head ref bucket id required but was unset")
	}

	// Build the SQL cursor from the selected head reference.
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

	// Connect commits to head-state persistence when a state store exists.
	var commitFn sql_mysql.CommitFn
	if stateStore != nil {
		commitFn = func(nref *bucket.ObjectRef) error {
			// Persist each committed head state in the configured object store.
			return c.writeHeadState(ctx, stateStore, nref)
		}
	}

	// Create configured databases before publishing the SQL store.
	mysql := sql_mysql.NewMysql(cursor, commitFn)
	createDBs := c.conf.GetCreateDbs()
	if len(createDBs) != 0 {
		tx, err := mysql.NewMysqlTransaction(ctx, true)
		if err != nil {
			return err
		}
		for _, dbName := range c.conf.GetCreateDbs() {
			_, err := tx.OpenDatabase(ctx, dbName, true)
			if err != nil {
				tx.Discard()
				return err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return errors.Wrap(err, "commit create dbs")
		}
	}

	// Publish the SQL store until controller shutdown.
	le.Info("sql store ready")
	var handle hydra_sql.SqlStore = mysql
	ctr.SetValue(&handle)
	<-rctx.Done()

	// Clear the published SQL store after controller shutdown.
	le.Debug("shutting down")
	ctr.SetValue(nil)
	return context.Canceled
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		serviceID := c.conf.GetSqlRpcServiceId()
		if serviceID != "" && serviceID == d.LookupRpcServiceID() {
			return directive.R(
				directive.NewGetterResolver(func(ctx context.Context) (bifrost_rpc.LookupRpcServiceValue, error) {
					store, err := c.GetSqlStore(ctx)
					if err != nil {
						return nil, err
					}
					mux := srpc.NewMux()
					handler := sql_rpc.NewSRPCSqlHandler(sql_rpc_server.NewStore(store), serviceID)
					if err := mux.Register(handler); err != nil {
						return nil, err
					}
					var invoker bifrost_rpc.LookupRpcServiceValue = mux
					return invoker, nil
				}),
				nil,
			)
		}
	}
	return c.Controller.HandleDirective(ctx, inst)
}
