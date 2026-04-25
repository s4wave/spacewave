package repair

import packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"

// ApplyUpdates returns entries with repaired metadata merged by pack id.
func ApplyUpdates(
	entries []*packfile.PackfileEntry,
	updates []*packfile.PackfileEntry,
) []*packfile.PackfileEntry {
	byID := make(map[string]*packfile.PackfileEntry, len(updates))
	for _, update := range updates {
		byID[update.GetId()] = update
	}

	out := make([]*packfile.PackfileEntry, 0, len(entries))
	for _, entry := range entries {
		if update := byID[entry.GetId()]; update != nil {
			out = append(out, update.CloneVT())
			continue
		}
		out = append(out, entry.CloneVT())
	}
	return out
}
