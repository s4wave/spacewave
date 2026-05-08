//go:build !js

package devtool

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tui "github.com/grindlemire/go-tui"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

// BuildDevtoolTUIDashboardElement constructs the static Bldr devtool dashboard tree.
func BuildDevtoolTUIDashboardElement(snapshot *devtool_status.BldrDevtoolStatus) *tui.Element {
	return buildDevtoolTUIDashboardElement(snapshot, 0)
}

func buildDevtoolTUIDashboardElement(snapshot *devtool_status.BldrDevtoolStatus, frame int) *tui.Element {
	if snapshot == nil {
		snapshot = devtool_status.EmptyBldrDevtoolStatus()
	}
	root := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Column),
		tui.WithPadding(1),
		tui.WithGap(1),
		devtoolTUITextStyle(tui.BrightWhite),
	)
	root.AddChild(buildDevtoolTUICommandHeader(snapshot, frame))
	root.AddChild(buildDevtoolTUIActivityPane(snapshot))
	root.AddChild(buildDevtoolTUIFooter(snapshot))
	return root
}

func buildDevtoolTUICommandHeader(snapshot *devtool_status.BldrDevtoolStatus, frame int) *tui.Element {
	command := snapshot.GetCommand()
	stateText := command.State.String()
	if command.Name != "" {
		stateText = command.Name + " " + stateText
	}
	lines := []string{
		"bldr " + devtoolTUIStatusMark(command.State) + " " + stateText,
		devtoolTUIProgressText(snapshot, frame),
		devtoolTUISummaryText(snapshot),
	}
	if command.Summary != "" {
		lines = append(lines, "summary: "+cleanTUIText(command.Summary))
	}
	if command.Error != "" {
		lines = append(lines, "error: "+cleanTUIText(command.Error))
	}
	return buildDevtoolTUIPanel("devtool", lines, devtoolTUIColor(tui.Cyan), tui.WithHeightAuto())
}

func buildDevtoolTUIActivityPane(snapshot *devtool_status.BldrDevtoolStatus) *tui.Element {
	color := devtoolTUIColor(tui.Green)
	if len(devtoolTUIErrorLines(snapshot)) > 0 {
		color = devtoolTUIColor(tui.BrightRed)
	}
	lines := devtoolTUIImportantLines(snapshot)
	if len(lines) == 0 {
		lines = append(lines, "clean - waiting for work")
	}
	return buildDevtoolTUIPanel("activity", lines, color, tui.WithFlexGrow(1))
}

func buildDevtoolTUIFooter(snapshot *devtool_status.BldrDevtoolStatus) *tui.Element {
	logFile := snapshot.GetCommand().LogFile
	if logFile == "" {
		logFile = ".bldr/logs"
	}
	lines := []string{
		"q quit  ctrl-c stop  logs " + compactDevtoolTUILogFile(logFile),
	}
	return buildDevtoolTUIPanel("controls", lines, devtoolTUIColor(tui.BrightBlack), tui.WithHeightAuto())
}

func buildDevtoolTUIPanel(title string, lines []string, color tui.Color, opts ...tui.Option) *tui.Element {
	panelOpts := []tui.Option{
		tui.WithBorder(tui.BorderSingle),
		tui.WithPaddingTRBL(0, 1, 0, 1),
		tui.WithOverflow(tui.OverflowHidden),
		tui.WithText(title + "\n" + strings.Join(lines, "\n")),
		devtoolTUITextStyle(color),
		devtoolTUIBorderStyle(color),
	}
	panelOpts = append(panelOpts, opts...)
	return tui.New(panelOpts...)
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
		return devtoolTUIProgressBar(command.State, frame, false) + " " + command.State.String()
	}
	entries := activeDevtoolTUIWorkEntries(snapshot)
	state := command.State
	bar := devtoolTUIProgressBar(state, frame, len(entries) > 0)
	if len(entries) > 0 {
		return bar + " " + strconv.Itoa(len(entries)) + " active"
	}
	return bar + " idle"
}

func devtoolTUIProgressBar(
	state devtool_status.BldrDevtoolCommandState,
	frame int,
	active bool,
) string {
	const width = 18
	const segment = 5

	if state == devtool_status.BldrDevtoolCommandStateDone {
		return "[" + strings.Repeat("=", width) + "]"
	}
	if state == devtool_status.BldrDevtoolCommandStateError {
		return "[" + strings.Repeat("!", width) + "]"
	}
	if !active {
		return "[" + strings.Repeat("-", width) + "]"
	}

	span := width - segment + 1
	start := max(frame%span, 0)
	var b strings.Builder
	b.WriteByte('[')
	for idx := range width {
		if idx >= start && idx < start+segment {
			if idx == start+segment-1 {
				b.WriteByte('>')
				continue
			}
			b.WriteByte('=')
			continue
		}
		b.WriteByte('-')
	}
	b.WriteByte(']')
	return b.String()
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

func devtoolTUIColor(color tui.Color) tui.Color {
	if !devtoolTUIShouldUseColor() {
		return tui.DefaultColor()
	}
	return color
}

func devtoolTUIBorderStyle(color tui.Color) tui.Option {
	return tui.WithBorderStyle(tui.NewStyle().Foreground(color))
}

func devtoolTUITextStyle(color tui.Color) tui.Option {
	return tui.WithTextStyle(tui.NewStyle().Foreground(devtoolTUIColor(color)))
}

func devtoolTUIShouldUseColor() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("CLICOLOR") == "0" {
		return false
	}
	return strings.ToLower(os.Getenv("TERM")) != "dumb"
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
