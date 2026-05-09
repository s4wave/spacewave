//go:build !js

package devtool

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

// BuildDevtoolTUIDashboard constructs the static Bldr devtool dashboard text.
func BuildDevtoolTUIDashboard(snapshot *devtool_status.BldrDevtoolStatus) string {
	return buildDevtoolTUIDashboard(snapshot, 0)
}

func buildDevtoolTUIDashboard(snapshot *devtool_status.BldrDevtoolStatus, frame int) string {
	if snapshot == nil {
		snapshot = devtool_status.EmptyBldrDevtoolStatus()
	}
	var lines []string
	lines = appendDevtoolTUISection(lines, "devtool", devtoolTUICommandHeaderLines(snapshot, frame))
	lines = appendDevtoolTUISection(lines, "activity", devtoolTUIActivityLines(snapshot))
	lines = appendDevtoolTUISection(lines, "controls", devtoolTUIFooterLines(snapshot))
	return strings.Join(lines, "\n") + "\n"
}

func devtoolTUICommandHeaderLines(snapshot *devtool_status.BldrDevtoolStatus, frame int) []string {
	command := snapshot.GetCommand()
	stateText := "state: " + devtoolTUIStatusMark(command.State)
	if command.Name != "" {
		stateText = "command: " + command.Name + " (" + devtoolTUIStatusMark(command.State) + ")"
	}
	lines := []string{
		stateText,
		devtoolTUIProgressText(snapshot, frame),
		devtoolTUISummaryText(snapshot),
	}
	if command.Summary != "" {
		lines = append(lines, "summary: "+cleanTUIText(command.Summary))
	}
	if command.Error != "" {
		lines = append(lines, "error: "+cleanTUIText(command.Error))
	}
	return lines
}

func devtoolTUIActivityLines(snapshot *devtool_status.BldrDevtoolStatus) []string {
	lines := devtoolTUIImportantLines(snapshot)
	if len(lines) == 0 {
		lines = append(lines, "clean - waiting for work")
	}
	return lines
}

func devtoolTUIFooterLines(snapshot *devtool_status.BldrDevtoolStatus) []string {
	logFile := snapshot.GetCommand().LogFile
	if logFile == "" {
		logFile = ".bldr/logs"
	}
	return []string{
		"ctrl-c stop  logs " + compactDevtoolTUILogFile(logFile),
	}
}

func appendDevtoolTUISection(dst []string, title string, lines []string) []string {
	if len(dst) > 0 {
		dst = append(dst, "")
	}
	dst = append(dst, title)
	for _, line := range lines {
		dst = append(dst, "  "+line)
	}
	return dst
}

func manifestBuildReasonText(row devtool_status.BldrDevtoolManifestBuildRow) string {
	switch {
	case row.Error != "":
		return row.Error
	case row.DependencyRebuildReason != "":
		return row.DependencyRebuildReason
	case row.CacheHit:
		return firstNonEmpty(row.Summary, "cache hit")
	case row.HotRebuild:
		return firstNonEmpty(row.Summary, "hot rebuild")
	case row.FullRebuild:
		return firstNonEmpty(row.Summary, "full rebuild")
	default:
		return row.Summary
	}
}

func manifestStateMark(state devtool_status.BldrDevtoolManifestState) string {
	switch state {
	case devtool_status.BldrDevtoolManifestStateQueued:
		return "wait"
	case devtool_status.BldrDevtoolManifestStateRunning:
		return "run"
	case devtool_status.BldrDevtoolManifestStateReady:
		return "ok"
	case devtool_status.BldrDevtoolManifestStateError:
		return "err"
	case devtool_status.BldrDevtoolManifestStateCanceled:
		return "cancel"
	default:
		return "?"
	}
}

func pluginSummaryText(row devtool_status.BldrDevtoolPluginRow) string {
	summary := firstNonEmpty(row.Error, row.Summary)
	if row.Error != "" && row.LastErrorAt != "" {
		return summary + " at " + row.LastErrorAt
	}
	return summary
}

func pluginStateMark(state devtool_status.BldrDevtoolPluginState) string {
	switch state {
	case devtool_status.BldrDevtoolPluginStateRequested:
		return "wait"
	case devtool_status.BldrDevtoolPluginStateRunning:
		return "run"
	case devtool_status.BldrDevtoolPluginStateErrored:
		return "err"
	default:
		return "?"
	}
}

func pluginTargetText(row devtool_status.BldrDevtoolPluginRow) string {
	if row.InstanceKey == "" {
		return row.PluginID
	}
	return row.PluginID + "/" + row.InstanceKey
}

func controllerStateMark(state devtool_status.BldrDevtoolControllerState) string {
	switch state {
	case devtool_status.BldrDevtoolControllerStateRequested:
		return "wait"
	case devtool_status.BldrDevtoolControllerStateRunning:
		return "run"
	case devtool_status.BldrDevtoolControllerStateIdle:
		return "ok"
	case devtool_status.BldrDevtoolControllerStateError:
		return "err"
	default:
		return "?"
	}
}

func devtoolTUIStatusMark(state devtool_status.BldrDevtoolCommandState) string {
	switch state {
	case devtool_status.BldrDevtoolCommandStateStarting:
		return "wait"
	case devtool_status.BldrDevtoolCommandStateRunning:
		return "run"
	case devtool_status.BldrDevtoolCommandStateDone:
		return "done"
	case devtool_status.BldrDevtoolCommandStateError:
		return "err"
	case devtool_status.BldrDevtoolCommandStateCanceled:
		return "cancel"
	default:
		return "?"
	}
}

func devtoolTUIProgressText(snapshot *devtool_status.BldrDevtoolStatus, frame int) string {
	command := snapshot.GetCommand()
	if command.IsTerminal() {
		return "work: " + command.State.String()
	}
	entries := activeDevtoolTUIWorkEntries(snapshot)
	if len(entries) > 0 {
		return "work: " + strconv.Itoa(len(entries)) + " active"
	}
	return "work: idle"
}

func devtoolTUISummaryText(snapshot *devtool_status.BldrDevtoolStatus) string {
	fetchReady, _, fetchErr := manifestFetchStateCounts(snapshot.GetManifestFetchRows())
	buildReady, _, buildErr := manifestBuildStateCounts(snapshot.GetManifestBuildRows())
	pluginRun, _, pluginErr := pluginStateCounts(snapshot.GetPluginRows())
	return "manifest " + strconv.Itoa(fetchReady) + "/" + strconv.Itoa(len(snapshot.GetManifestFetchRows())) +
		" fetched, " + strconv.Itoa(buildReady) + "/" + strconv.Itoa(len(snapshot.GetManifestBuildRows())) +
		" built" +
		" | active " + strconv.Itoa(len(activeDevtoolTUIWorkEntries(snapshot))) +
		" | plugins " + strconv.Itoa(pluginRun) + " ok, " + strconv.Itoa(pluginErr) + " err" +
		" | attention " + strconv.Itoa(len(snapshot.GetAttentionRows())+fetchErr+buildErr)
}

func manifestFetchStateCounts(rows []devtool_status.BldrDevtoolManifestFetchRow) (ready, active, errored int) {
	for _, row := range rows {
		switch row.State {
		case devtool_status.BldrDevtoolManifestStateReady:
			ready++
		case devtool_status.BldrDevtoolManifestStateQueued, devtool_status.BldrDevtoolManifestStateRunning:
			active++
		case devtool_status.BldrDevtoolManifestStateError:
			errored++
		}
	}
	return ready, active, errored
}

func manifestBuildStateCounts(rows []devtool_status.BldrDevtoolManifestBuildRow) (ready, active, errored int) {
	for _, row := range rows {
		switch row.State {
		case devtool_status.BldrDevtoolManifestStateReady:
			ready++
		case devtool_status.BldrDevtoolManifestStateQueued, devtool_status.BldrDevtoolManifestStateRunning:
			active++
		case devtool_status.BldrDevtoolManifestStateError:
			errored++
		}
	}
	return ready, active, errored
}

func pluginStateCounts(rows []devtool_status.BldrDevtoolPluginRow) (running, waiting, errored int) {
	for _, row := range rows {
		switch row.State {
		case devtool_status.BldrDevtoolPluginStateRunning:
			running++
		case devtool_status.BldrDevtoolPluginStateRequested:
			waiting++
		case devtool_status.BldrDevtoolPluginStateErrored:
			errored++
		}
	}
	return running, waiting, errored
}

func devtoolTUIImportantLines(snapshot *devtool_status.BldrDevtoolStatus) []string {
	var lines []string
	for _, line := range devtoolTUIErrorLines(snapshot) {
		lines = append(lines, line)
		if len(lines) == 3 {
			return lines
		}
	}
	for _, line := range devtoolTUIActiveLines(snapshot) {
		lines = append(lines, line)
		if len(lines) == 4 {
			return lines
		}
	}
	return lines
}

func devtoolTUIErrorLines(snapshot *devtool_status.BldrDevtoolStatus) []string {
	var lines []string
	if command := snapshot.GetCommand(); command.Error != "" {
		lines = append(lines, "err command "+shortTUIText(command.Error, 84))
	}
	for _, row := range sortedDevtoolTUIManifestFetchRows(snapshot) {
		if row.Error != "" {
			lines = append(lines, "err fetch "+shortTUIText(firstNonEmpty(row.ManifestID, row.ID)+" "+row.Error, 84))
		}
	}
	for _, row := range sortedDevtoolTUIManifestBuildRows(snapshot) {
		if row.Error != "" {
			lines = append(lines, "err build "+shortTUIText(firstNonEmpty(row.BuildTargets, row.ManifestID, row.ID)+" "+row.Error, 84))
		}
	}
	for _, row := range sortedDevtoolTUIPluginRows(snapshot) {
		if row.Error != "" {
			lines = append(lines, "err plugin "+shortTUIText(pluginTargetText(row)+" "+pluginSummaryText(row), 84))
		}
	}
	for _, row := range sortedDevtoolTUIControllerRows(snapshot) {
		if row.Error != "" {
			lines = append(lines, "err controller "+shortTUIText(firstNonEmpty(row.ControllerID, row.ID)+" "+row.Error, 84))
		}
	}
	for _, row := range sortedDevtoolTUIAttentionRows(snapshot) {
		lines = append(lines, row.Severity.String()+" "+shortTUIText(firstNonEmpty(row.Source, "attention")+" "+firstNonEmpty(row.Message, row.Detail), 84))
	}
	return lines
}

func devtoolTUIActiveLines(snapshot *devtool_status.BldrDevtoolStatus) []string {
	var lines []string
	if command := snapshot.GetCommand(); command.Name != "" && !command.IsTerminal() {
		lines = append(lines, devtoolTUIStatusMark(command.State)+" command "+shortTUIText(command.Name+" "+command.Summary, 84))
	}
	for _, row := range sortedDevtoolTUIManifestBuildRows(snapshot) {
		if row.State == devtool_status.BldrDevtoolManifestStateQueued ||
			row.State == devtool_status.BldrDevtoolManifestStateRunning {
			lines = append(lines, manifestStateMark(row.State)+" build "+shortTUIText(firstNonEmpty(row.BuildTargets, row.ManifestID, row.ID)+" "+manifestBuildReasonText(row), 84))
		}
	}
	for _, row := range sortedDevtoolTUIManifestFetchRows(snapshot) {
		if row.State == devtool_status.BldrDevtoolManifestStateQueued ||
			row.State == devtool_status.BldrDevtoolManifestStateRunning {
			lines = append(lines, manifestStateMark(row.State)+" fetch "+shortTUIText(firstNonEmpty(row.ManifestID, row.ID)+" "+firstNonEmpty(row.Summary, row.PlatformID), 84))
		}
	}
	for _, row := range sortedDevtoolTUIPluginRows(snapshot) {
		if row.State == devtool_status.BldrDevtoolPluginStateRequested ||
			row.State == devtool_status.BldrDevtoolPluginStateRunning {
			lines = append(lines, pluginStateMark(row.State)+" plugin "+shortTUIText(pluginTargetText(row)+" "+row.Summary, 84))
		}
	}
	for _, row := range sortedDevtoolTUIControllerRows(snapshot) {
		if row.State == devtool_status.BldrDevtoolControllerStateRequested ||
			row.State == devtool_status.BldrDevtoolControllerStateRunning {
			lines = append(lines, controllerStateMark(row.State)+" controller "+shortTUIText(firstNonEmpty(row.ControllerID, row.ID)+" "+row.Summary, 84))
		}
	}
	return lines
}

func activeDevtoolTUIWorkEntries(snapshot *devtool_status.BldrDevtoolStatus) []string {
	var entries []string
	command := snapshot.GetCommand()
	if command.Name != "" && !command.IsTerminal() {
		entries = append(entries, command.Name+" "+command.State.String())
	}
	for _, row := range sortedDevtoolTUIManifestFetchRows(snapshot) {
		if row.State == devtool_status.BldrDevtoolManifestStateQueued ||
			row.State == devtool_status.BldrDevtoolManifestStateRunning {
			entries = append(entries, "fetch "+firstNonEmpty(row.ManifestID, row.ID)+" "+row.State.String())
		}
	}
	for _, row := range sortedDevtoolTUIManifestBuildRows(snapshot) {
		if row.State == devtool_status.BldrDevtoolManifestStateQueued ||
			row.State == devtool_status.BldrDevtoolManifestStateRunning {
			entries = append(entries, "build "+firstNonEmpty(row.BuildTargets, row.ManifestID, row.ID)+" "+row.State.String())
		}
	}
	for _, row := range sortedDevtoolTUIPluginRows(snapshot) {
		if row.State == devtool_status.BldrDevtoolPluginStateRequested ||
			row.State == devtool_status.BldrDevtoolPluginStateRunning {
			entries = append(entries, "plugin "+firstNonEmpty(row.PluginID, row.ID)+" "+row.State.String())
		}
	}
	for _, row := range sortedDevtoolTUIControllerRows(snapshot) {
		if row.State == devtool_status.BldrDevtoolControllerStateRequested ||
			row.State == devtool_status.BldrDevtoolControllerStateRunning {
			entries = append(entries, "controller "+firstNonEmpty(row.ControllerID, row.ID)+" "+row.State.String())
		}
	}
	return entries
}

func sortedDevtoolTUIManifestFetchRows(
	snapshot *devtool_status.BldrDevtoolStatus,
) []devtool_status.BldrDevtoolManifestFetchRow {
	rows := snapshot.GetManifestFetchRows()
	slices.SortFunc(rows, func(a, b devtool_status.BldrDevtoolManifestFetchRow) int {
		return strings.Compare(a.ID, b.ID)
	})
	return rows
}

func sortedDevtoolTUIManifestBuildRows(
	snapshot *devtool_status.BldrDevtoolStatus,
) []devtool_status.BldrDevtoolManifestBuildRow {
	rows := snapshot.GetManifestBuildRows()
	slices.SortFunc(rows, func(a, b devtool_status.BldrDevtoolManifestBuildRow) int {
		return strings.Compare(a.ID, b.ID)
	})
	return rows
}

func sortedDevtoolTUIPluginRows(
	snapshot *devtool_status.BldrDevtoolStatus,
) []devtool_status.BldrDevtoolPluginRow {
	rows := snapshot.GetPluginRows()
	slices.SortFunc(rows, func(a, b devtool_status.BldrDevtoolPluginRow) int {
		return strings.Compare(a.ID, b.ID)
	})
	return rows
}

func sortedDevtoolTUIControllerRows(
	snapshot *devtool_status.BldrDevtoolStatus,
) []devtool_status.BldrDevtoolControllerRow {
	rows := snapshot.GetControllerRows()
	slices.SortFunc(rows, func(a, b devtool_status.BldrDevtoolControllerRow) int {
		return strings.Compare(a.ID, b.ID)
	})
	return rows
}

func sortedDevtoolTUIAttentionRows(
	snapshot *devtool_status.BldrDevtoolStatus,
) []devtool_status.BldrDevtoolAttentionRow {
	rows := snapshot.GetAttentionRows()
	slices.SortFunc(rows, func(a, b devtool_status.BldrDevtoolAttentionRow) int {
		return strings.Compare(a.ID, b.ID)
	})
	return rows
}

func compactDevtoolTUILogFile(logFile string) string {
	logFile = cleanTUIText(logFile)
	if logFile == "" {
		return ""
	}
	cleaned := filepath.Clean(logFile)
	bldrPart := string(filepath.Separator) + ".bldr" + string(filepath.Separator)
	if idx := strings.LastIndex(cleaned, bldrPart); idx >= 0 {
		return ".bldr" + string(filepath.Separator) + cleaned[idx+len(bldrPart):]
	}
	if strings.HasPrefix(cleaned, ".bldr"+string(filepath.Separator)) {
		return cleaned
	}
	return shortTUIText(cleaned, 72)
}

func shortTUIText(value string, maxLen int) string {
	value = cleanTUIText(value)
	if maxLen < 1 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

func cleanTUIText(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\t", " ")
	return strings.Join(strings.Fields(value), " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
