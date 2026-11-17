//go:build !js && !wasip1

package world_recovery

import (
	"encoding/hex"
	"io"
	"strconv"
	"strings"
)

// WriteReport prints the count table with its durable state and coverage.
func WriteReport(writer io.Writer, result *Result) error {
	// Keep provisional state visible before any count can be mistaken for a total.
	state := "provisional"
	if result.GetPhase() == Phase_PHASE_COUNTED {
		state = "counted within declared coverage"
	}
	if result.GetPhase() == Phase_PHASE_STALE {
		state = "stale; start a new scratch file"
	}

	// Render safe capture boundaries before the count table.
	var output strings.Builder
	output.WriteString("Recovery count: " + state + "\n")
	output.WriteString("Capture: " + result.GetInput().GetCaptureId() + "\nFile: " + result.GetInput().GetPath() + "\nTables: " + strings.Join(result.GetInput().GetTables(), ", ") + "\nInput: " + hex.EncodeToString(result.GetInputIdentity()) + "\n")
	if result.GetFailure() != "" {
		output.WriteString("Stopped: " + result.GetFailure() + "\n")
	}
	count := result.GetCounts()
	for _, row := range []struct {
		label string
		value uint64
	}{
		{"Physical locations", count.GetPhysicalLocations()},
		{"Distinct blocks", count.GetPhysicalBlocks()},
		{"Distinct bytes", count.GetPhysicalBytes()},
		{"Proven orphan blocks", count.GetOrphanBlocks()},
		{"Proven orphan bytes", count.GetOrphanBytes()},
		{"Reachability unknown", count.GetUnknownReachability()},
		{"Attributed object versions", count.GetObjectVersions()},
		{"Candidate object keys", count.GetCandidateKeys()},
		{"Attributable absent versions", count.GetAttributableVersions()},
		{"Changed keys", count.GetChangedKeys()},
		{"Complete candidate closures", count.GetRecoverableVersions()},
		{"Partial candidate closures", count.GetPartialVersions()},
		{"Confirmed refusal losses", count.GetConfirmedRefusalLosses()},
		{"Versions with observed deletes", count.GetIntentionalDeletes()},
		{"Unbound structural candidates", count.GetStructuralCandidates()},
		{"Unassigned blocks", count.GetUnassignedBlocks()},
		{"Live missing blocks", count.GetLiveHoles()},
		{"Pack files read", count.GetPackFiles()},
		{"Unreadable pack files", count.GetUnreadablePacks()},
		{"Unread pack tail bytes", count.GetUnreadTailBytes()},
		{"Corrupt pack values", count.GetCorruptValues()},
	} {
		output.WriteString(row.label)
		output.WriteString(strings.Repeat(" ", 32-len(row.label)))
		output.WriteString(strconv.FormatUint(row.value, 10) + "\n")
	}

	// Display every bounded source and interpretation gap beside the result.
	output.WriteString("Coverage: immutable SQLite Volume records")
	for _, p := range result.GetInput().GetPacks() {
		tier := "local pack log"
		if p.GetTier() == PackTier_PACK_TIER_REMOTE {
			tier = "retained remote packs"
		}
		output.WriteString(", " + tier + " " + p.GetPath())
	}
	output.WriteString(" and captured typed Worlds\n")
	output.WriteString("Confirmed losses and versions at observed delete keys are subsets of attributable absent versions.\nObserved deletes are key-level evidence, not proof each older exact version was intentionally deleted.\nUnbound candidates have no proven Space membership. Zero confirmed does not prove zero loss.\n")
	for _, gap := range result.GetGaps() {
		output.WriteString("Unknown: " + gap + "\n")
	}
	_, err := io.WriteString(writer, output.String())
	return err
}
