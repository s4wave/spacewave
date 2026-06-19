//go:build !tinygo && !sql_lite

package s4wave_sql_workbench_world

import (
	"context"
	"slices"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql_query "github.com/s4wave/spacewave/sdk/sql/query"
	s4wave_sql_query_result "github.com/s4wave/spacewave/sdk/sql/query-result"
	s4wave_sql_workbench "github.com/s4wave/spacewave/sdk/sql/workbench"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
)

// SqlWorkbenchResource serves SqlWorkbenchResourceService for one SQL workbench object.
type SqlWorkbenchResource struct {
	ws        world.WorldState
	objectKey string
	mux       srpc.Mux
}

// NewSqlWorkbenchResource constructs a SQL workbench resource.
func NewSqlWorkbenchResource(
	ws world.WorldState,
	objectKey string,
) *SqlWorkbenchResource {
	r := &SqlWorkbenchResource{
		ws:        ws,
		objectKey: objectKey,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_sql_workbench.SRPCRegisterSqlWorkbenchResourceService(mux, r)
	})
	return r
}

// GetMux returns the SRPC mux for this resource.
func (r *SqlWorkbenchResource) GetMux() srpc.Mux {
	return r.mux
}

// Close releases the resource lifecycle.
func (r *SqlWorkbenchResource) Close() {}

// GetWorkbench returns the persisted workbench state.
func (r *SqlWorkbenchResource) GetWorkbench(
	ctx context.Context,
	_ *s4wave_sql_workbench.GetWorkbenchRequest,
) (*s4wave_sql_workbench.GetWorkbenchResponse, error) {
	workbench, err := r.readWorkbench(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_sql_workbench.GetWorkbenchResponse{Workbench: workbench.CloneVT()}, nil
}

// AddPin pins a sql/query object.
func (r *SqlWorkbenchResource) AddPin(
	ctx context.Context,
	req *s4wave_sql_workbench.AddPinRequest,
) (*s4wave_sql_workbench.AddPinResponse, error) {
	queryKey := req.GetQueryObjectKey()
	if queryKey == "" {
		return nil, errors.New("sql/workbench: query object key is required")
	}
	workbench, err := r.readWorkbench(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.validatePinnedQuery(ctx, workbench, queryKey); err != nil {
		return nil, err
	}
	next := workbench.CloneVT()
	if !containsString(next.GetPinnedQueryObjectKeys(), queryKey) {
		next.PinnedQueryObjectKeys = append(next.PinnedQueryObjectKeys, queryKey)
	}
	if err := r.commitWorkbenchRoot(ctx, next); err != nil {
		return nil, err
	}
	return &s4wave_sql_workbench.AddPinResponse{}, nil
}

// RemovePin removes a pinned sql/query object.
func (r *SqlWorkbenchResource) RemovePin(
	ctx context.Context,
	req *s4wave_sql_workbench.RemovePinRequest,
) (*s4wave_sql_workbench.RemovePinResponse, error) {
	queryKey := req.GetQueryObjectKey()
	if queryKey == "" {
		return nil, errors.New("sql/workbench: query object key is required")
	}
	workbench, err := r.readWorkbench(ctx)
	if err != nil {
		return nil, err
	}
	next := workbench.CloneVT()
	next.PinnedQueryObjectKeys = removeString(next.GetPinnedQueryObjectKeys(), queryKey)
	if err := r.commitWorkbenchRoot(ctx, next); err != nil {
		return nil, err
	}
	return &s4wave_sql_workbench.RemovePinResponse{}, nil
}

// SetLayout replaces the open tabs and layout preferences.
func (r *SqlWorkbenchResource) SetLayout(
	ctx context.Context,
	req *s4wave_sql_workbench.SetLayoutRequest,
) (*s4wave_sql_workbench.SetLayoutResponse, error) {
	workbench, err := r.readWorkbench(ctx)
	if err != nil {
		return nil, err
	}
	tabs := cloneWorkbenchTabs(req.GetOpenTabs())
	if err := r.validateOpenTabs(ctx, tabs); err != nil {
		return nil, err
	}
	next := workbench.CloneVT()
	next.OpenTabs = tabs
	next.Layout = nil
	if req.GetLayout() != nil {
		next.Layout = req.GetLayout().CloneVT()
	}
	if err := r.commitWorkbenchRoot(ctx, next); err != nil {
		return nil, err
	}
	return &s4wave_sql_workbench.SetLayoutResponse{}, nil
}

func (r *SqlWorkbenchResource) readWorkbench(ctx context.Context) (*s4wave_sql_workbench.Workbench, error) {
	if r.ws == nil {
		return nil, errors.New("sql/workbench: world state is required")
	}
	if err := world_types.CheckObjectType(ctx, r.ws, r.objectKey, s4wave_sql_workbench.SqlWorkbenchTypeID); err != nil {
		return nil, err
	}
	return s4wave_sql_workbench.ReadWorkbenchRoot(ctx, r.ws, r.objectKey)
}

func (r *SqlWorkbenchResource) validatePinnedQuery(
	ctx context.Context,
	workbench *s4wave_sql_workbench.Workbench,
	queryKey string,
) error {
	if err := world_types.CheckObjectType(ctx, r.ws, queryKey, s4wave_sql_query.SqlQueryTypeID); err != nil {
		return err
	}
	query, err := s4wave_sql_query.ReadQueryRoot(ctx, r.ws, queryKey)
	if err != nil {
		return err
	}
	targetKey := workbench.GetTargetDbObjectKey()
	if targetKey == "" || query.GetTargetDbObjectKey() == "" || query.GetTargetDbObjectKey() == targetKey {
		return nil
	}
	return errors.Errorf("sql/workbench: query target %s does not match workbench target %s", query.GetTargetDbObjectKey(), targetKey)
}

func (r *SqlWorkbenchResource) validateOpenTabs(
	ctx context.Context,
	tabs []*s4wave_sql_workbench.WorkbenchTab,
) error {
	for _, tab := range tabs {
		if tab == nil || tab.GetObjectKey() == "" {
			continue
		}
		switch tab.GetKind() {
		case s4wave_sql_workbench.WorkbenchTabKind_WORKBENCH_TAB_KIND_QUERY:
			if err := world_types.CheckObjectType(ctx, r.ws, tab.GetObjectKey(), s4wave_sql_query.SqlQueryTypeID); err != nil {
				return err
			}
		case s4wave_sql_workbench.WorkbenchTabKind_WORKBENCH_TAB_KIND_QUERY_RESULT:
			if err := world_types.CheckObjectType(ctx, r.ws, tab.GetObjectKey(), s4wave_sql_query_result.SqlQueryResultTypeID); err != nil {
				return err
			}
		default:
			return errors.Errorf("sql/workbench: unsupported tab kind %s", tab.GetKind().String())
		}
	}
	return nil
}

func (r *SqlWorkbenchResource) commitWorkbenchRoot(
	ctx context.Context,
	workbench *s4wave_sql_workbench.Workbench,
) error {
	if targetKey := workbench.GetTargetDbObjectKey(); targetKey != "" {
		if err := world_types.CheckObjectType(ctx, r.ws, targetKey, s4wave_sql_world.SqlDbTypeID); err != nil {
			return err
		}
	}
	rootRef, err := s4wave_sql_workbench.WriteWorkbenchRootRef(ctx, r.ws, workbench)
	if err != nil {
		return err
	}
	_, sysErr, err := r.ws.ApplyWorldOp(ctx, NewSqlWorkbenchSetRootOp(r.objectKey, rootRef), "")
	if err != nil {
		return err
	}
	if sysErr {
		return errors.New("sql/workbench: root update returned a system error")
	}
	return nil
}

func cloneWorkbenchTabs(tabs []*s4wave_sql_workbench.WorkbenchTab) []*s4wave_sql_workbench.WorkbenchTab {
	if len(tabs) == 0 {
		return nil
	}
	cloned := make([]*s4wave_sql_workbench.WorkbenchTab, 0, len(tabs))
	for _, tab := range tabs {
		if tab != nil {
			cloned = append(cloned, tab.CloneVT())
		}
	}
	return cloned
}

func containsString(values []string, target string) bool {
	return slices.Contains(values, target)
}

func removeString(values []string, target string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:0]
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

// _ is a type assertion.
var _ s4wave_sql_workbench.SRPCSqlWorkbenchResourceServiceServer = (*SqlWorkbenchResource)(nil)
