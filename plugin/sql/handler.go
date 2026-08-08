package sql_plugin

import (
	"context"
	"database/sql/driver"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	s4wave_sql "github.com/s4wave/spacewave/db/sql"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
	s4wave_sql_query "github.com/s4wave/spacewave/sdk/sql/query"
	s4wave_sql_query_result "github.com/s4wave/spacewave/sdk/sql/query-result"
	s4wave_sql_query_result_world "github.com/s4wave/spacewave/sdk/sql/query-result/world"
	s4wave_sql_query_world "github.com/s4wave/spacewave/sdk/sql/query/world"
	s4wave_sql_schema "github.com/s4wave/spacewave/sdk/sql/schema"
	s4wave_sql_schema_world "github.com/s4wave/spacewave/sdk/sql/schema/world"
	s4wave_sql_table_view "github.com/s4wave/spacewave/sdk/sql/table-view"
	s4wave_sql_table_view_world "github.com/s4wave/spacewave/sdk/sql/table-view/world"
	s4wave_sql_workbench "github.com/s4wave/spacewave/sdk/sql/workbench"
	s4wave_sql_workbench_world "github.com/s4wave/spacewave/sdk/sql/workbench/world"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	s4wave_viewer_registry "github.com/s4wave/spacewave/sdk/viewer/registry"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
	"github.com/sirupsen/logrus"
)

// SQLQuickstartID is the app-visible SQL quickstart id.
const SQLQuickstartID = "sql"

const (
	sqlQuickstartDBKey    = "sql/db"
	sqlQuickstartQueryKey = "sql/query/example"
)

var sqlObjectTypeIDs = []string{
	s4wave_sql_world.SqlDbTypeID,
	s4wave_sql_query.SqlQueryTypeID,
	s4wave_sql_query_result.SqlQueryResultTypeID,
	s4wave_sql_schema.SqlSchemaTypeID,
	s4wave_sql_table_view.SqlTableViewTypeID,
	s4wave_sql_workbench.SqlWorkbenchTypeID,
}

var sqlWorldOpIDs = []string{
	s4wave_sql_world.SqlSetRootOpId,
	s4wave_sql_query_world.SqlQuerySetRootOpId,
	s4wave_sql_query_result_world.SqlQueryResultSetRootOpId,
	s4wave_sql_schema_world.SqlSchemaSetRootOpId,
	s4wave_sql_table_view_world.SqlTableViewSetRootOpId,
	s4wave_sql_workbench_world.SqlWorkbenchSetRootOpId,
}

var sqlViewerRegistrations []*s4wave_viewer_registry.ViewerRegistration

// SQLHandler serves SQL plugin ObjectType, WorldOp, and Quickstart RPCs.
type SQLHandler struct {
	le *logrus.Entry
	b  bus.Bus
}

// InvokeObjectType opens a SQL ObjectType resource through the attached Engine.
func (h *SQLHandler) InvokeObjectType(
	ctx context.Context,
	req *s4wave_objecttype_registry.InvokeObjectTypeRequest,
) (*s4wave_objecttype_registry.InvokeObjectTypeResponse, error) {
	if req.GetAttachedEngineResourceId() == 0 {
		return nil, errors.New("sql plugin: attached engine resource is required")
	}
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	engine, err := sdk_world_engine.NewAttachedEngine(ctx, req.GetAttachedEngineResourceId())
	if err != nil {
		return nil, err
	}
	ws := world.NewEngineWorldState(engine, true)
	invoker, cleanup, err := h.openObjectType(ctx, req.GetTypeId(), engine, ws, req.GetObjectKey())
	if err != nil {
		engine.Release()
		return nil, err
	}
	resourceID, err := resourceCtx.AddResource(invoker, func() {
		cleanup()
		engine.Release()
	})
	if err != nil {
		cleanup()
		engine.Release()
		return nil, err
	}
	return &s4wave_objecttype_registry.InvokeObjectTypeResponse{ResourceId: resourceID}, nil
}

func (h *SQLHandler) openObjectType(
	ctx context.Context,
	typeID string,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	switch typeID {
	case s4wave_sql_world.SqlDbTypeID:
		return s4wave_sql_world.SqlDbFactory(ctx, h.le, h.b, engine, ws, objectKey)
	case s4wave_sql_query.SqlQueryTypeID:
		return s4wave_sql_query_world.SqlQueryFactory(ctx, h.le, h.b, engine, ws, objectKey)
	case s4wave_sql_query_result.SqlQueryResultTypeID:
		return s4wave_sql_query_result_world.SqlQueryResultFactory(ctx, h.le, h.b, engine, ws, objectKey)
	case s4wave_sql_schema.SqlSchemaTypeID:
		return s4wave_sql_schema_world.SqlSchemaFactory(ctx, h.le, h.b, engine, ws, objectKey)
	case s4wave_sql_table_view.SqlTableViewTypeID:
		return s4wave_sql_table_view_world.SqlTableViewFactory(ctx, h.le, h.b, engine, ws, objectKey)
	case s4wave_sql_workbench.SqlWorkbenchTypeID:
		return s4wave_sql_workbench_world.SqlWorkbenchFactory(ctx, h.le, h.b, engine, ws, objectKey)
	default:
		return nil, nil, errors.Errorf("sql plugin: unhandled object type %s", typeID)
	}
}

// ApplyWorldOp applies a SQL world operation through the attached WorldState.
func (h *SQLHandler) ApplyWorldOp(
	ctx context.Context,
	req *s4wave_worldop_registry.ApplyWorldOpRequest,
) (*s4wave_worldop_registry.ApplyWorldOpResponse, error) {
	if req.GetAttachedWorldStateResourceId() == 0 {
		return nil, errors.New("sql plugin: attached world state resource is required")
	}
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	client := sdk_world_engine.NewAttachedResourceClient(resourceCtx)
	ref := client.CreateResourceReference(req.GetAttachedWorldStateResourceId())
	ws, err := sdk_world_engine.NewSDKWorldState(client, ref, false)
	if err != nil {
		ref.Release()
		return nil, err
	}
	defer ws.Release()

	op, err := h.unmarshalSQLOp(ctx, req.GetOperationTypeId(), req.GetOpData())
	if err != nil {
		return nil, err
	}
	sysErr, err := op.ApplyWorldOp(ctx, h.le, ws, peer.ID(""))
	if err != nil {
		return nil, err
	}
	return &s4wave_worldop_registry.ApplyWorldOpResponse{SystemError: sysErr}, nil
}

// ApplyWorldObjectOp applies a SQL operation through the attached ObjectState.
func (h *SQLHandler) ApplyWorldObjectOp(
	ctx context.Context,
	req *s4wave_worldop_registry.ApplyWorldObjectOpRequest,
) (*s4wave_worldop_registry.ApplyWorldObjectOpResponse, error) {
	if req.GetAttachedObjectStateResourceId() == 0 {
		return nil, errors.New("sql plugin: attached object state resource is required")
	}
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	client := sdk_world_engine.NewAttachedResourceClient(resourceCtx)
	ref := client.CreateResourceReference(req.GetAttachedObjectStateResourceId())
	objectClient, err := ref.GetClient()
	if err != nil {
		ref.Release()
		return nil, err
	}
	objectSvc := s4wave_world.NewSRPCObjectStateResourceServiceClient(objectClient)
	keyResp, err := objectSvc.GetKey(ctx, &s4wave_world.GetKeyRequest{})
	if err != nil {
		ref.Release()
		return nil, err
	}
	objectKey := keyResp.GetObjectKey()
	if req.GetObjectKey() != objectKey {
		ref.Release()
		return nil, errors.Errorf("sql plugin: attached object key %q does not match request key %q", objectKey, req.GetObjectKey())
	}
	os, err := sdk_world_engine.NewSDKObjectState(client, ref, objectKey)
	if err != nil {
		ref.Release()
		return nil, err
	}
	defer os.Release()

	op, err := h.unmarshalSQLOp(ctx, req.GetOperationTypeId(), req.GetOpData())
	if err != nil {
		return nil, err
	}
	sysErr, err := op.ApplyWorldObjectOp(ctx, h.le, os, peer.ID(""))
	if err != nil {
		return nil, err
	}
	return &s4wave_worldop_registry.ApplyWorldObjectOpResponse{SystemError: sysErr}, nil
}

// ValidateOp validates a SQL operation payload.
func (h *SQLHandler) ValidateOp(
	ctx context.Context,
	req *s4wave_worldop_registry.ValidateOpRequest,
) (*s4wave_worldop_registry.ValidateOpResponse, error) {
	op, err := h.unmarshalSQLOp(ctx, req.GetOperationTypeId(), req.GetOpData())
	if err != nil {
		return &s4wave_worldop_registry.ValidateOpResponse{Error: err.Error()}, nil
	}
	if err := op.Validate(); err != nil {
		return &s4wave_worldop_registry.ValidateOpResponse{Error: err.Error()}, nil
	}
	return &s4wave_worldop_registry.ValidateOpResponse{}, nil
}

func (h *SQLHandler) unmarshalSQLOp(ctx context.Context, opTypeID string, data []byte) (world.Operation, error) {
	op, err := lookupSQLWorldOp(ctx, opTypeID)
	if err != nil {
		return nil, errors.Wrapf(err, "lookup SQL world op %s", opTypeID)
	}
	if op == nil {
		return nil, errors.Errorf("lookup SQL world op %s: unhandled operation type", opTypeID)
	}
	if err := op.UnmarshalBlock(data); err != nil {
		return nil, err
	}
	return op, nil
}

func lookupSQLWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		s4wave_sql_world.LookupSqlSetRootOp,
		s4wave_sql_query_world.LookupSqlQuerySetRootOp,
		s4wave_sql_query_result_world.LookupSqlQueryResultSetRootOp,
		s4wave_sql_schema_world.LookupSqlSchemaSetRootOp,
		s4wave_sql_table_view_world.LookupSqlTableViewSetRootOp,
		s4wave_sql_workbench_world.LookupSqlWorkbenchSetRootOp,
	}).LookupOp(ctx, opTypeID)
}

func lookupSQLBlockType(typeID string) (blocktype.BlockType, error) {
	switch typeID {
	case s4wave_sql_query.SqlQueryBlockTypeID:
		return s4wave_sql_query.SqlQueryBlockType, nil
	case s4wave_sql_workbench.SqlWorkbenchBlockTypeID:
		return s4wave_sql_workbench.SqlWorkbenchBlockType, nil
	default:
		return nil, nil
	}
}

// SeedQuickstart seeds the SQL quickstart in the attached Space world.
func (h *SQLHandler) SeedQuickstart(
	ctx context.Context,
	req *s4wave_quickstart_registry.SeedQuickstartRequest,
) (*s4wave_quickstart_registry.SeedQuickstartResponse, error) {
	if req.GetQuickstartId() != SQLQuickstartID {
		return nil, errors.Errorf("sql plugin: unhandled quickstart %s", req.GetQuickstartId())
	}
	if req.GetAttachedEngineResourceId() == 0 {
		return nil, errors.New("sql plugin: attached engine resource is required")
	}
	engine, err := sdk_world_engine.NewAttachedEngine(ctx, req.GetAttachedEngineResourceId())
	if err != nil {
		return nil, errors.Wrap(err, "sql plugin: attach quickstart engine")
	}
	defer engine.Release()

	ws := world.NewEngineWorldState(engine, true)
	if err := h.seedSQLQuickstart(ctx, ws); err != nil {
		return nil, errors.Wrap(err, "sql plugin: seed quickstart")
	}
	return &s4wave_quickstart_registry.SeedQuickstartResponse{
		IndexPath: sqlQuickstartDBKey,
		PluginIds: []string{PluginID},
	}, nil
}

func (h *SQLHandler) seedSQLQuickstart(ctx context.Context, ws world.WorldState) error {
	if _, err := ws.CreateObject(ctx, sqlQuickstartDBKey, nil); err != nil {
		return errors.Wrap(err, "create SQL database object")
	}
	if err := h.seedSQLDatabase(ctx, ws); err != nil {
		return errors.Wrap(err, "seed SQL database")
	}
	if err := world_types.SetObjectType(ctx, ws, sqlQuickstartDBKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		return errors.Wrap(err, "set SQL database object type")
	}
	query := &s4wave_sql_query.Query{
		SqlText:           "SELECT name, role FROM quickstart.people WHERE id = ?",
		DialectHint:       "mysql",
		TargetDbObjectKey: sqlQuickstartDBKey,
		Parameters: []*s4wave_sql.SqlValue{{
			Value: &s4wave_sql.SqlValue_IntValue{IntValue: 1},
		}},
	}
	_, _, err := world.CreateWorldObject(ctx, ws, sqlQuickstartQueryKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(query, true)
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "create SQL query object")
	}
	if err := world_types.SetObjectType(ctx, ws, sqlQuickstartQueryKey, s4wave_sql_query.SqlQueryTypeID); err != nil {
		return errors.Wrap(err, "set SQL query object type")
	}
	return nil
}

func (h *SQLHandler) seedSQLDatabase(ctx context.Context, ws world.WorldState) error {
	obj, err := world.MustGetObject(ctx, ws, sqlQuickstartDBKey)
	if err != nil {
		return errors.Wrap(err, "open SQL database object")
	}

	var committedRoot *bucket.ObjectRef
	if err := obj.AccessWorldState(ctx, nil, func(root *bucket_lookup.Cursor) error {
		sqlRoot := root.Clone()
		defer sqlRoot.Release()
		store := sql_mysql.NewMysql(sqlRoot, func(next *bucket.ObjectRef) error {
			committedRoot = next.Clone()
			return nil
		})
		tx, err := store.NewMysqlTransaction(ctx, true)
		if err != nil {
			return errors.Wrap(err, "open quickstart schema transaction")
		}
		if _, err := tx.OpenDatabase(ctx, "quickstart", true); err != nil {
			tx.Discard()
			return errors.Wrap(err, "create quickstart schema")
		}
		if err := tx.Commit(ctx); err != nil {
			return errors.Wrap(err, "commit quickstart schema")
		}
		return execSQLTx(ctx, store, "/quickstart", []string{
			"CREATE TABLE people (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL, role TEXT NOT NULL)",
			"INSERT INTO people (id, name, role) VALUES (1, 'ada', 'analyst')",
			"INSERT INTO people (id, name, role) VALUES (2, 'grace', 'engineer')",
			"CREATE TABLE projects (id BIGINT NOT NULL PRIMARY KEY, owner_id BIGINT NOT NULL, title TEXT NOT NULL)",
			"INSERT INTO projects (id, owner_id, title) VALUES (10, 1, 'difference engine notes')",
			"INSERT INTO projects (id, owner_id, title) VALUES (11, 2, 'compiler logbook')",
		})
	}); err != nil {
		return errors.Wrap(err, "build SQL database root")
	}
	if committedRoot == nil || committedRoot.GetEmpty() {
		return errors.New("sql plugin: quickstart SQL root is empty")
	}
	if _, err := obj.SetRootRef(ctx, committedRoot); err != nil {
		return errors.Wrap(err, "commit SQL database root")
	}
	return nil
}

func execSQLTx(ctx context.Context, store s4wave_sql.SqlStore, dsn string, statements []string) error {
	tx, err := store.NewSqlTransaction(ctx, true, dsn)
	if err != nil {
		return err
	}
	defer tx.Discard()
	ops, err := tx.GetSqlOps(ctx)
	if err != nil {
		return err
	}
	for _, statement := range statements {
		if _, err := ops.ExecContext(ctx, statement, []driver.NamedValue{}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// _ is a type assertion.
var _ s4wave_objecttype_registry.SRPCObjectTypeHandlerServiceServer = (*SQLHandler)(nil)

// _ is a type assertion.
var _ s4wave_worldop_registry.SRPCWorldOpHandlerServiceServer = (*SQLHandler)(nil)

// _ is a type assertion.
var _ s4wave_quickstart_registry.SRPCQuickstartHandlerServiceServer = (*SQLHandler)(nil)
