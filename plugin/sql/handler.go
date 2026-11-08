package sql_plugin

import (
	"context"
	"database/sql/driver"

	"github.com/aperturerobotics/common/typeid"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	s4wave_sql "github.com/s4wave/spacewave/db/sql"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
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
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
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
	s4wave_sql_world.SqlSetRootOpTypeID,
	s4wave_sql_query_world.SqlQuerySetRootOpTypeID,
	s4wave_sql_query_result_world.SqlQueryResultSetRootOpTypeID,
	s4wave_sql_schema_world.SqlSchemaSetRootOpTypeID,
	s4wave_sql_table_view_world.SqlTableViewSetRootOpTypeID,
	s4wave_sql_workbench_world.SqlWorkbenchSetRootOpTypeID,
}

var sqlViewerRegistrations []*s4wave_viewer_registry.ViewerRegistration

// SQLHandler serves SQL plugin ObjectType and Quickstart RPCs.
type SQLHandler struct {
	le *logrus.Entry
	b  bus.Bus
}

// InvokeObjectType opens a SQL ObjectType resource through the attached Engine.
func (h *SQLHandler) InvokeObjectType(
	ctx context.Context,
	req *s4wave_objecttype_registry.InvokeObjectTypeRequest,
) (*s4wave_objecttype_registry.InvokeObjectTypeResponse, error) {
	// Require an attached Engine before opening a SQL resource.
	if req.GetAttachedEngineResourceId() == 0 {
		return nil, errors.New("sql plugin: attached engine resource is required")
	}

	// Resolve the resource client context for the SQL response.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Attach the Engine and open the requested SQL object type.
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

	// Register the SQL resource with cleanup for its attached Engine.
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

// sqlSchema declares the SQL world operations and block types.
var sqlSchema = world.NewSchema(
	s4wave_sql_world.WorldTypes,
	s4wave_sql_query_world.WorldTypes,
	s4wave_sql_query_result_world.WorldTypes,
	s4wave_sql_schema_world.WorldTypes,
	s4wave_sql_table_view_world.WorldTypes,
	s4wave_sql_workbench_world.WorldTypes,
	s4wave_sql_query.QueryTypes,
	s4wave_sql_query_result.QueryResultTypes,
	s4wave_sql_schema.SchemaTypes,
	s4wave_sql_table_view.TableViewTypes,
	s4wave_sql_workbench.WorkbenchTypes,
	[]typeid.Type{{ID: s4wave_sql_world.SqlDbTypeID, New: func() any { return new(sql_mysql.Root) }}},
)

// SeedQuickstart seeds the SQL quickstart in the attached Space world.
func (h *SQLHandler) SeedQuickstart(
	ctx context.Context,
	req *s4wave_quickstart_registry.SeedQuickstartRequest,
) (*s4wave_quickstart_registry.SeedQuickstartResponse, error) {
	// Require the SQL quickstart and its attached Engine.
	if req.GetQuickstartId() != SQLQuickstartID {
		return nil, errors.Errorf("sql plugin: unhandled quickstart %s", req.GetQuickstartId())
	}
	if req.GetAttachedEngineResourceId() == 0 {
		return nil, errors.New("sql plugin: attached engine resource is required")
	}

	// Attach the quickstart Engine until seeding completes.
	engine, err := sdk_world_engine.NewAttachedEngine(ctx, req.GetAttachedEngineResourceId())
	if err != nil {
		return nil, errors.Wrap(err, "sql plugin: attach quickstart engine")
	}
	defer engine.Release()

	// Seed the SQL database and example query in the attached WorldState.
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
	// Create the SQL database object in the quickstart WorldState.
	{
		createdObject, err := ws.CreateObject(ctx, sqlQuickstartDBKey, nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			return errors.Wrap(err, "create SQL database object")
		}
	}

	// Seed the SQL database and assign its object type.
	if err := h.seedSQLDatabase(ctx, ws); err != nil {
		return errors.Wrap(err, "seed SQL database")
	}
	if err := world_types.SetObjectType(ctx, ws, sqlQuickstartDBKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		return errors.Wrap(err, "set SQL database object type")
	}

	// Create the example query against the quickstart database.
	query := &s4wave_sql_query.Query{
		SqlText:           "SELECT name, role FROM quickstart.people WHERE id = ?",
		DialectHint:       "mysql",
		TargetDbObjectKey: sqlQuickstartDBKey,
		Parameters: []*s4wave_sql.SqlValue{{
			Value: &s4wave_sql.SqlValue_IntValue{IntValue: 1},
		}},
	}
	createdObject2, _, err := world.CreateWorldObject(ctx, ws, sqlQuickstartQueryKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(query, true)
		return nil
	})
	world.ReleaseObjectState(createdObject2)
	if err != nil {
		return errors.Wrap(err, "create SQL query object")
	}

	// Assign the SQL query object type to the example query.
	if err := world_types.SetObjectType(ctx, ws, sqlQuickstartQueryKey, s4wave_sql_query.SqlQueryTypeID); err != nil {
		return errors.Wrap(err, "set SQL query object type")
	}
	return nil
}

// seedSQLDatabase creates the quickstart schema and its sample rows in the SQL
// database object.
func (h *SQLHandler) seedSQLDatabase(ctx context.Context, ws world.WorldState) error {
	// Open the SQL database object for quickstart seeding.
	obj, err := world.MustGetObject(ctx, ws, sqlQuickstartDBKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return errors.Wrap(err, "open SQL database object")
	}

	// Build the schema through a stage held until the object adopts the root.
	rootRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return errors.Wrap(err, "read SQL database root")
	}
	stage, err := ws.StageWorldState(ctx)
	if err != nil {
		return errors.Wrap(err, "stage SQL database root")
	}
	defer stage.Release()
	var committedRoot *bucket.ObjectRef
	if err := stage.AccessWorldState(ctx, rootRef, func(root *bucket_lookup.Cursor) error {
		// Open the MySQL store on a clone of the staged root.
		sqlRoot := root.Clone()
		defer sqlRoot.Release()
		store := sql_mysql.NewMysql(sqlRoot, func(next *bucket.ObjectRef) error {
			committedRoot = next.Clone()
			return nil
		})
		defer store.Close()

		// Create and commit the quickstart database schema.
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

	// Commit the populated SQL root to the database object.
	if committedRoot == nil || committedRoot.GetEmpty() {
		return errors.New("sql plugin: quickstart SQL root is empty")
	}
	if _, err := obj.SetRootRef(ctx, committedRoot); err != nil {
		return errors.Wrap(err, "commit SQL database root")
	}
	return nil
}

func execSQLTx(ctx context.Context, store s4wave_sql.SqlStore, dsn string, statements []string) error {
	// Open a writable SQL transaction for the requested database.
	tx, err := store.NewSqlTransaction(ctx, true, dsn)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Execute the supplied SQL statements through the transaction.
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
var _ s4wave_quickstart_registry.SRPCQuickstartHandlerServiceServer = (*SQLHandler)(nil)
