package resource_layout

import (
	"context"
	"path"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
)

// SetLayoutModelFunc is called when the frontend wants to update the layout model.
type SetLayoutModelFunc = func(ctx context.Context, layoutModel *s4wave_layout.LayoutModel) error

// NavigateTabFunc is called when the frontend wants to navigate within a tab.
type NavigateTabFunc = func(ctx context.Context, navigateTabReq *s4wave_layout.NavigateTabRequest) (*s4wave_layout.NavigateTabResponse, error)

// AddTabFunc is called when the frontend wants to add a new tab.
type AddTabFunc = func(ctx context.Context, req *s4wave_layout.AddTabRequest) (*s4wave_layout.AddTabResponse, error)

// LayoutResource wraps layout host functionality for resource access.
type LayoutResource struct {
	mux         srpc.Invoker
	stateCtr    ccontainer.Watchable[*s4wave_layout.LayoutModel]
	setLayout   SetLayoutModelFunc
	navigateTab NavigateTabFunc
	addTab      AddTabFunc
}

// NewLayoutResource creates a new LayoutResource.
//
// stateCtr is a watchable container for the layout model state.
// setLayout is called when the frontend updates the layout (may be nil).
// navigateTab is called when the frontend navigates within a tab (may be nil).
func NewLayoutResource(
	stateCtr ccontainer.Watchable[*s4wave_layout.LayoutModel],
	setLayout SetLayoutModelFunc,
	navigateTab NavigateTabFunc,
) *LayoutResource {
	r := &LayoutResource{
		stateCtr:    stateCtr,
		setLayout:   setLayout,
		navigateTab: navigateTab,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_layout.SRPCRegisterLayoutHost(mux, r)
	})
	return r
}

// SetAddTabFunc sets the callback for adding tabs.
func (r *LayoutResource) SetAddTabFunc(addTab AddTabFunc) {
	r.addTab = addTab
}

// GetMux returns the rpc mux.
func (r *LayoutResource) GetMux() srpc.Invoker {
	return r.mux
}

// WatchLayoutModel watches the LayoutModel.
func (r *LayoutResource) WatchLayoutModel(strm s4wave_layout.SRPCLayoutHost_WatchLayoutModelStream) error {
	errCh := make(chan error, 2)
	go func() {
		for {
			req, err := strm.Recv()
			if err != nil {
				errCh <- err
				return
			}
			switch b := req.GetBody().(type) {
			case *s4wave_layout.WatchLayoutModelRequest_SetModel:
				if r.setLayout != nil {
					err := r.setLayout(strm.Context(), b.SetModel)
					if err != nil {
						errCh <- err
						return
					}
				}
			}
		}
	}()
	return ccontainer.WatchChanges(strm.Context(), nil, r.stateCtr, strm.Send, errCh)
}

// NavigateTab navigates within a tab.
func (r *LayoutResource) NavigateTab(ctx context.Context, req *s4wave_layout.NavigateTabRequest) (*s4wave_layout.NavigateTabResponse, error) {
	if r.navigateTab != nil {
		return r.navigateTab(ctx, req)
	}
	return &s4wave_layout.NavigateTabResponse{}, nil
}

// AddTab adds a new tab to the layout.
func (r *LayoutResource) AddTab(ctx context.Context, req *s4wave_layout.AddTabRequest) (*s4wave_layout.AddTabResponse, error) {
	if r.addTab != nil {
		return r.addTab(ctx, req)
	}
	return &s4wave_layout.AddTabResponse{}, nil
}

// WalkLayoutModel walks all nodes in a layout model, calling fn for each node.
// Returns early if fn returns false.
func WalkLayoutModel(m *s4wave_layout.LayoutModel, fn func(node any) bool) {
	if m == nil {
		return
	}
	for _, border := range m.GetBorders() {
		if !fn(border) {
			return
		}
		for _, tab := range border.GetChildren() {
			if !fn(tab) {
				return
			}
		}
	}
	walkRowDef(m.GetLayout(), fn)
}

// walkRowDef walks a row definition and its children.
func walkRowDef(row *s4wave_layout.RowDef, fn func(node any) bool) {
	if row == nil {
		return
	}
	if !fn(row) {
		return
	}
	for _, child := range row.GetChildren() {
		if !fn(child) {
			return
		}
		switch node := child.GetNode().(type) {
		case *s4wave_layout.RowOrTabSetDef_Row:
			walkRowDef(node.Row, fn)
		case *s4wave_layout.RowOrTabSetDef_TabSet:
			if !fn(node.TabSet) {
				return
			}
			for _, tab := range node.TabSet.GetChildren() {
				if !fn(tab) {
					return
				}
			}
		}
	}
}

// CleanupPath normalizes a path, joining it with basePath if relative.
func CleanupPath(basePath, targetPath string) string {
	if !path.IsAbs(targetPath) {
		targetPath = path.Join(basePath, targetPath)
	}
	return path.Clean(targetPath)
}

// _ is a type assertion
var _ s4wave_layout.SRPCLayoutHostServer = ((*LayoutResource)(nil))
