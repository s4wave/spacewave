//go:build !js

package spacewave_cli

import (
	"strconv"

	"github.com/s4wave/spacewave/db/volume/restore"
)

// volumeRefillFields formats the counts, with conditional labels for a dry run.
func volumeRefillFields(r restore.RefillResult, dryRun bool) [][2]string {
	// Use the same outcome order in both modes.
	labels := [6]string{"Already stored", "Source lacks", "Hash mismatch", "Restored", "Restored under a hold, parked", "Dropped"}
	bytesLabel := "Bytes restored"
	if dryRun {
		labels = [6]string{
			"Would skip (already stored)", "Would lack from source", "Would reject (hash mismatch)",
			"Would restore", "Would restore under a hold, parked", "Would drop",
		}
		bytesLabel = "Bytes that would be restored"
	}

	// Count each input ref exactly once in the totals.
	fields := make([][2]string, 1, len(labels)+2)
	var total uint64
	for i, label := range labels {
		fields = append(fields, [2]string{label, strconv.FormatUint(r.Counts[i], 10)})
		total += r.Counts[i]
	}
	fields[0] = [2]string{"Refs read", strconv.FormatUint(total, 10)}
	return append(fields, [2]string{bytesLabel, strconv.FormatUint(r.Bytes, 10)})
}
