package spacepolicy

import (
	"slices"
	"strconv"

	"github.com/aperturerobotics/util/ccontainer"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
)

const maxProjectedSpaces = 5

// Row contains one session space row to project into desktop navigation.
type Row struct {
	SessionIndex uint32
	SessionLabel string
	Space        *space.SpaceSoListEntry
}

// Build maps session space rows into desktop runtime navigation items.
func Build(rows []*Row) []*desktop_runtime.DesktopRuntimeNavigationItem {
	out := make([]*desktop_runtime.DesktopRuntimeNavigationItem, 0, min(len(rows), maxProjectedSpaces))
	for _, row := range rows {
		if row == nil || row.Space == nil || len(out) >= maxProjectedSpaces {
			continue
		}
		id := SpaceID(row.Space)
		if id == "" {
			continue
		}
		out = append(out, &desktop_runtime.DesktopRuntimeNavigationItem{
			Id:         "space-" + strconv.FormatUint(uint64(row.SessionIndex), 10) + "-" + id,
			Label:      label(row.Space),
			Detail:     detail(row),
			Route:      sessionRoute(row.SessionIndex) + "so/" + id,
			StatusText: statusText(row.Space),
		})
	}
	return out
}

// ReadSnapshot reads the current shared-object list without waiting for the
// first non-nil value; the caller owns watching for future changes.
func ReadSnapshot(
	soListWatchable ccontainer.Watchable[*sobject.SharedObjectList],
	excludedBlockStoreIDs ...string,
) (*sobject.SharedObjectList, []*space.SpaceSoListEntry, error) {
	soList := soListWatchable.GetValue()
	spaces, err := BuildSessionSpaces(soList, excludedBlockStoreIDs...)
	if err != nil {
		return nil, nil, err
	}
	return soList, spaces, nil
}

// BuildSessionSpaces maps a SharedObjectList to visible Space entries.
func BuildSessionSpaces(
	soList *sobject.SharedObjectList,
	excludedBlockStoreIDs ...string,
) ([]*space.SpaceSoListEntry, error) {
	if soList == nil {
		return nil, nil
	}
	spaces, err := space.FilterSharedObjectList(
		soList.GetSharedObjects(),
		func(_ *sobject.SharedObjectListEntry, _ error) error {
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	out := spaces[:0]
	for _, sp := range spaces {
		if slices.Contains(excludedBlockStoreIDs, sp.GetEntry().GetRef().GetBlockStoreId()) {
			continue
		}
		out = append(out, sp)
	}
	return out, nil
}

// SpaceID returns the provider resource id for a projected Space.
func SpaceID(sp *space.SpaceSoListEntry) string {
	return sp.GetEntry().GetRef().GetProviderResourceRef().GetId()
}

func label(sp *space.SpaceSoListEntry) string {
	if sp.GetSpaceMeta().GetName() != "" {
		return sp.GetSpaceMeta().GetName()
	}
	if SpaceID(sp) != "" {
		return SpaceID(sp)
	}
	return "Untitled space"
}

func detail(row *Row) string {
	source := statusText(row.Space)
	if row.SessionLabel == "" {
		return source
	}
	if source == "" {
		return row.SessionLabel
	}
	return row.SessionLabel + " - " + source
}

func statusText(sp *space.SpaceSoListEntry) string {
	switch sp.GetEntry().GetSource() {
	case "created":
		return "Created"
	case "shared":
		return "Shared"
	default:
		return "Available"
	}
}

func sessionRoute(sessionIndex uint32) string {
	return "/u/" + strconv.FormatUint(uint64(sessionIndex), 10) + "/"
}
