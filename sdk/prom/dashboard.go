package s4wave_prom

import (
	"context"
	"maps"
	"slices"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// ErrPanelRemoved is returned when an edit references a panel that another
// writer removed.
var ErrPanelRemoved = errors.New("another member removed this panel")

// ErrPanelTitleRequired is returned when a stored panel has no title.
var ErrPanelTitleRequired = errors.New("a panel needs a title")

// CreateDashboard creates the World object key as a prom/dashboard holding
// dash.
func CreateDashboard(ctx context.Context, ws world.WorldState, key string, dash *Dashboard) error {
	obj, _, err := world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(dash, true)
		return nil
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		return errors.Wrapf(err, "create dashboard %q", key)
	}
	return world_types.SetObjectType(ctx, ws, key, DashboardTypeID)
}

// ReadDashboard reads the Dashboard key.
func ReadDashboard(ctx context.Context, ws world.WorldState, key string) (*Dashboard, error) {
	// Decode the object's root block.
	obj, err := world.MustGetObject(ctx, ws, key)
	if err != nil {
		return nil, err
	}
	defer world.ReleaseObjectState(obj)
	var dash *Dashboard
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		dash, err = block.UnmarshalBlock[Dashboard](ctx, bcs)
		return err
	})
	if err != nil {
		return nil, errors.Wrapf(err, "read dashboard %q", key)
	}

	// A Dashboard without a root has no panels.
	if dash == nil {
		dash = &Dashboard{}
	}
	return dash, nil
}

// UpdateDashboard applies edit to the Dashboard key as ws holds it and stores
// the result. Run it inside a write transaction so the read and the write see
// the same Dashboard.
func UpdateDashboard(ctx context.Context, ws world.WorldState, key string, edit func(*Dashboard) error) error {
	// Read the Dashboard and apply the edit.
	dash, err := ReadDashboard(ctx, ws, key)
	if err != nil {
		return err
	}
	if err := edit(dash); err != nil {
		return err
	}

	// Store the edited Dashboard as the object's root.
	obj, err := world.MustGetObject(ctx, ws, key)
	if err != nil {
		return err
	}
	defer world.ReleaseObjectState(obj)
	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(dash, true)
		return nil
	})
	return errors.Wrapf(err, "write dashboard %q", key)
}

// PutPanel adds panel under a new id when its id is empty, or replaces the
// panel with its id. It returns the stored panel's id.
func (d *Dashboard) PutPanel(panel *Panel) (string, error) {
	// Require a title, as the editor does.
	if panel.GetTitle() == "" {
		return "", ErrPanelTitleRequired
	}

	// Add a new panel at the end.
	panel = panel.CloneVT()
	if panel.GetId() == "" {
		panel.Id = ulid.NewULID()
		d.Panels = append(d.Panels, panel)
		return panel.GetId(), nil
	}

	// Replace the panel in place.
	i := d.panelIndex(panel.GetId())
	if i < 0 {
		return "", ErrPanelRemoved
	}
	d.Panels[i] = panel
	return panel.GetId(), nil
}

// RemovePanel removes the panel id, if present.
func (d *Dashboard) RemovePanel(id string) {
	d.Panels = slices.DeleteFunc(d.Panels, func(p *Panel) bool { return p.GetId() == id })
}

// MovePanel moves the panel id to index, clamped to the last position.
func (d *Dashboard) MovePanel(id string, index int) error {
	// Find the panel.
	i := d.panelIndex(id)
	if i < 0 {
		return ErrPanelRemoved
	}

	// Take it out and insert it at index.
	panel := d.Panels[i]
	d.Panels = slices.Delete(d.Panels, i, i+1)
	d.Panels = slices.Insert(d.Panels, min(index, len(d.Panels)), panel)
	return nil
}

// panelIndex returns the position of the panel id, or -1.
func (d *Dashboard) panelIndex(id string) int {
	return slices.IndexFunc(d.Panels, func(p *Panel) bool { return p.GetId() == id })
}

// starterPanels holds the starter panels of each job, in display order.
var starterPanels = map[string][]*Panel{
	"node": {
		{
			Title: "CPU busy",
			Expr:  `100 * (1 - avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])))`,
			Unit:  PanelUnit_PANEL_UNIT_PERCENT,
			Kind:  PanelKind_PANEL_KIND_CHART,
		},
		{
			Title: "Memory used",
			Expr:  `node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes`,
			Unit:  PanelUnit_PANEL_UNIT_BYTES,
			Kind:  PanelKind_PANEL_KIND_CHART,
		},
		{
			Title: "Filesystem used",
			Expr:  `100 * (1 - node_filesystem_avail_bytes{fstype!~"tmpfs|overlay"} / node_filesystem_size_bytes{fstype!~"tmpfs|overlay"})`,
			Unit:  PanelUnit_PANEL_UNIT_PERCENT,
			Kind:  PanelKind_PANEL_KIND_VALUE,
		},
		{
			Title: "Network received",
			Expr:  `sum by (instance) (rate(node_network_receive_bytes_total{device!="lo"}[5m]))`,
			Unit:  PanelUnit_PANEL_UNIT_BYTES_PER_SECOND,
			Kind:  PanelKind_PANEL_KIND_CHART,
		},
	},
	"smartctl": {
		{
			Title: "Drive temperature",
			Expr:  `smartctl_device_temperature{temperature_type="current"}`,
			Unit:  PanelUnit_PANEL_UNIT_CELSIUS,
			Kind:  PanelKind_PANEL_KIND_VALUE,
		},
	},
	"spacewave": {
		{
			Title: "Heap in use",
			Expr:  `go_memstats_heap_inuse_bytes{job="spacewave"}`,
			Unit:  PanelUnit_PANEL_UNIT_BYTES,
			Kind:  PanelKind_PANEL_KIND_CHART,
		},
		{
			Title: "Goroutines",
			Expr:  `go_goroutines{job="spacewave"}`,
			Kind:  PanelKind_PANEL_KIND_CHART,
		},
	},
}

// StarterDashboard returns a Dashboard with the starter panels of each job in
// jobs that has them, in the order node, smartctl, spacewave.
func StarterDashboard(jobs []string) *Dashboard {
	// Index the jobs.
	has := make(map[string]bool, len(jobs))
	for _, job := range jobs {
		has[job] = true
	}

	// Clone each present job's panels under new ids.
	dash := &Dashboard{}
	for _, job := range []string{"node", "smartctl", "spacewave"} {
		if !has[job] {
			continue
		}
		for _, panel := range starterPanels[job] {
			panel = panel.CloneVT()
			panel.Id = ulid.NewULID()
			dash.Panels = append(dash.Panels, panel)
		}
	}
	return dash
}

// TargetJobs returns the sorted job labels of the World's targets.
func TargetJobs(ctx context.Context, ws world.WorldState) ([]string, error) {
	// Collect each target's job label.
	keys, err := ListTargets(ctx, ws)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	for _, key := range keys {
		meta, _, err := ReadTarget(ctx, ws, key)
		if err != nil {
			return nil, err
		}
		for _, l := range meta.GetLabels().GetLabels() {
			if l.GetName() == "job" {
				seen[l.GetValue()] = true
			}
		}
	}

	// Return them sorted.
	return slices.Sorted(maps.Keys(seen)), nil
}
