package s4wave_sql_workbench

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	s4wave_sql "github.com/s4wave/spacewave/sdk/sql"
)

// SqlWorkbenchTypeID is the world ObjectType id for SQL workbenches.
const SqlWorkbenchTypeID = "sql/workbench"

// SqlWorkbenchBlockTypeID is the block type id for SQL workbench roots.
const SqlWorkbenchBlockTypeID = "github.com/s4wave/spacewave/sdk/sql/workbench.Workbench"

// SqlWorkbenchBlockType constructs SQL workbench root blocks for typed cursor writes.
var SqlWorkbenchBlockType = blocktype.NewBlockType(
	SqlWorkbenchBlockTypeID,
	func() *Workbench { return &Workbench{} },
)

// NewWorkbenchBlock constructs a SQL workbench block.
func NewWorkbenchBlock() block.Block {
	return &Workbench{}
}

// ReadWorkbenchRoot reads a SQL workbench object's root.
func ReadWorkbenchRoot(ctx context.Context, ws world.WorldState, objectKey string) (*Workbench, error) {
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		return nil, err
	}
	return ReadWorkbenchObjectRoot(ctx, obj)
}

// ReadWorkbenchObjectRoot reads a SQL workbench root from an object state.
func ReadWorkbenchObjectRoot(ctx context.Context, obj world.ObjectState) (*Workbench, error) {
	var workbench *Workbench
	_, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		workbench, err = block.UnmarshalBlock[*Workbench](ctx, bcs, NewWorkbenchBlock)
		if err != nil {
			return err
		}
		if workbench == nil {
			return world.ErrObjectNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return workbench, nil
}

// WriteWorkbenchRootRef writes a SQL workbench root block and returns its ref.
func WriteWorkbenchRootRef(ctx context.Context, ws world.WorldState, workbench *Workbench) (*bucket.ObjectRef, error) {
	return world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs.SetBlock(workbench, true)
		return nil
	})
}

// SyncWorkbenchGraphQuads replaces the workbench's SQL graph links.
func SyncWorkbenchGraphQuads(ctx context.Context, ws world.WorldState, objectKey string) error {
	workbench, err := ReadWorkbenchRoot(ctx, ws, objectKey)
	if err != nil {
		return err
	}
	for _, pred := range []string{
		s4wave_sql.PredSqlWorkbenchAgainstDb.String(),
		s4wave_sql.PredSqlWorkbenchPinnedQuery.String(),
		s4wave_sql.PredSqlWorkbenchOpenTab.String(),
	} {
		quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(objectKey, pred, "", ""), 0)
		if err != nil {
			return err
		}
		for _, q := range quads {
			if err := ws.DeleteGraphQuad(ctx, q); err != nil {
				return err
			}
		}
	}
	if workbench.GetTargetDbObjectKey() != "" {
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
			objectKey,
			s4wave_sql.PredSqlWorkbenchAgainstDb.String(),
			workbench.GetTargetDbObjectKey(),
			"",
		)); err != nil {
			return err
		}
	}
	for _, queryKey := range workbench.GetPinnedQueryObjectKeys() {
		if queryKey == "" {
			continue
		}
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
			objectKey,
			s4wave_sql.PredSqlWorkbenchPinnedQuery.String(),
			queryKey,
			"",
		)); err != nil {
			return err
		}
	}
	for _, tab := range workbench.GetOpenTabs() {
		if tab.GetObjectKey() == "" {
			continue
		}
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
			objectKey,
			s4wave_sql.PredSqlWorkbenchOpenTab.String(),
			tab.GetObjectKey(),
			"",
		)); err != nil {
			return err
		}
	}
	return nil
}

// MarshalBlock marshals the SQL workbench root.
func (w *Workbench) MarshalBlock() ([]byte, error) {
	return w.MarshalVT()
}

// UnmarshalBlock unmarshals the SQL workbench root.
func (w *Workbench) UnmarshalBlock(data []byte) error {
	return w.UnmarshalVT(data)
}

// Validate performs cursory checks on the SQL workbench root.
func (w *Workbench) Validate() error {
	return nil
}

var _ block.Block = (*Workbench)(nil)
