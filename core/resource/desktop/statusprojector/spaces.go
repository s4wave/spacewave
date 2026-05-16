package statusprojector

import (
	"strconv"

	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	"github.com/s4wave/spacewave/core/space"
)

const maxProjectedSpaces = 5

type spaceProjectionRow struct {
	sessionIndex uint32
	sessionLabel string
	space        *space.SpaceSoListEntry
}

func buildSpaceProjection(rows []*spaceProjectionRow) []*desktop_runtime.DesktopRuntimeNavigationItem {
	out := make([]*desktop_runtime.DesktopRuntimeNavigationItem, 0, min(len(rows), maxProjectedSpaces))
	for _, row := range rows {
		if row == nil || row.space == nil || len(out) >= maxProjectedSpaces {
			continue
		}
		id := spaceID(row.space)
		if id == "" {
			continue
		}
		out = append(out, &desktop_runtime.DesktopRuntimeNavigationItem{
			Id:         "space-" + strconv.FormatUint(uint64(row.sessionIndex), 10) + "-" + id,
			Label:      spaceLabel(row.space),
			Detail:     spaceDetail(row),
			Route:      sessionRoute(row.sessionIndex) + "so/" + id,
			StatusText: spaceStatusText(row.space),
		})
	}
	return out
}

func spaceID(sp *space.SpaceSoListEntry) string {
	return sp.GetEntry().GetRef().GetProviderResourceRef().GetId()
}

func spaceLabel(sp *space.SpaceSoListEntry) string {
	if sp.GetSpaceMeta().GetName() != "" {
		return sp.GetSpaceMeta().GetName()
	}
	if spaceID(sp) != "" {
		return spaceID(sp)
	}
	return "Untitled space"
}

func spaceDetail(row *spaceProjectionRow) string {
	source := spaceStatusText(row.space)
	if row.sessionLabel == "" {
		return source
	}
	if source == "" {
		return row.sessionLabel
	}
	return row.sessionLabel + " - " + source
}

func spaceStatusText(sp *space.SpaceSoListEntry) string {
	switch sp.GetEntry().GetSource() {
	case "created":
		return "Created"
	case "shared":
		return "Shared"
	default:
		return "Available"
	}
}
