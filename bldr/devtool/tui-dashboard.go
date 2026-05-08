//go:build !js

package devtool

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tui "github.com/grindlemire/go-tui"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

// BuildDevtoolTUIDashboardElement constructs the static Bldr devtool dashboard tree.
func BuildDevtoolTUIDashboardElement(snapshot *devtool_status.BldrDevtoolStatus) *tui.Element {
	if snapshot == nil {
		snapshot = devtool_status.EmptyBldrDevtoolStatus()
	}
	root := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Column),
		tui.WithPadding(1),
		tui.WithGap(1),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.White)),
	)
	root.AddChild(buildDevtoolTUICommandHeader(snapshot))

	body := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Column),
		tui.WithGap(1),
		tui.WithFlexGrow(1),
		tui.WithMinHeight(0),
	)
	body.AddChild(buildDevtoolTUIManifestPane(snapshot), buildDevtoolTUIPluginPane(snapshot))
	root.AddChild(body)

	root.AddChild(buildDevtoolTUIAttentionPane(snapshot))
	root.AddChild(buildDevtoolTUIFooter(snapshot))
	return root
}

func buildDevtoolTUICommandHeader(snapshot *devtool_status.BldrDevtoolStatus) *tui.Element {
	command := snapshot.GetCommand()
	lines := []string{
		"bldr devtool",
		"command: " + emptyDash(command.Name) +
			"  state: " + command.State.String() +
			"  manifests: " + strconv.Itoa(len(snapshot.GetManifestFetchRows())) +
			" fetch / " + strconv.Itoa(len(snapshot.GetManifestBuildRows())) +
			" build  plugins: " + strconv.Itoa(len(snapshot.GetPluginRows())) +
			"  attention: " + strconv.Itoa(len(snapshot.GetAttentionRows())),
	}
	if command.Summary != "" {
		lines = append(lines, "summary: "+cleanTUIText(command.Summary))
	}
	if current := currentDevtoolTUIWorkText(snapshot); current != "" {
		lines = append(lines, "current: "+current)
	}
	if command.LogFile != "" {
		lines = append(lines, "log: "+compactDevtoolTUILogFile(command.LogFile))
	}
	if command.Error != "" {
		lines = append(lines, "error: "+cleanTUIText(command.Error))
	}
	return buildDevtoolTUIPanel("Command", lines, tui.WithHeightAuto())
}

func buildDevtoolTUIManifestPane(snapshot *devtool_status.BldrDevtoolStatus) *tui.Element {
	lines := []string{"Fetch"}
	fetchRows := sortedDevtoolTUIManifestFetchRows(snapshot)
	if len(fetchRows) == 0 {
		lines = append(lines, "  no manifest fetches")
	}
	if len(fetchRows) > 0 {
		lines = append(lines, "  state     manifest                platform       refs     summary")
		for _, row := range fetchRows {
			lines = append(lines, "  "+rowText(
				tuiColumn{value: row.State.String(), width: 9},
				tuiColumn{value: row.ManifestID, width: 22},
				tuiColumn{value: row.PlatformID, width: 14},
				tuiColumn{value: manifestFetchReadyText(row), width: 8},
				tuiColumn{value: manifestFetchSummaryText(row), width: 42},
			))
		}
	}

	lines = append(lines, "", "Build")
	buildRows := sortedDevtoolTUIManifestBuildRows(snapshot)
	if len(buildRows) == 0 {
		lines = append(lines, "  no manifest builds")
	}
	if len(buildRows) > 0 {
		lines = append(lines, "  state     target       platform       type     mode    watched reason/error")
		for _, row := range buildRows {
			lines = append(lines, "  "+rowText(
				tuiColumn{value: row.State.String(), width: 9},
				tuiColumn{value: firstNonEmpty(row.BuildTargets, row.ManifestID), width: 12},
				tuiColumn{value: firstNonEmpty(row.PlatformID, row.TargetPlatformIDs), width: 14},
				tuiColumn{value: row.BuildType, width: 8},
				tuiColumn{value: manifestBuildModeText(row), width: 7},
				tuiColumn{value: watchedFileCountText(row.WatchedFileCount), width: 7},
				tuiColumn{value: manifestBuildReasonText(row), width: 34},
			))
		}
	}
	return buildDevtoolTUIPanel("Manifest Fetch / Build", lines, tui.WithFlexGrow(2))
}

func buildDevtoolTUIPluginPane(snapshot *devtool_status.BldrDevtoolStatus) *tui.Element {
	rows := sortedDevtoolTUIPluginRows(snapshot)
	lines := []string{pluginHealthSummaryText(rows)}
	if len(rows) == 0 {
		lines = append(lines, "no plugins requested")
	}
	if len(rows) > 0 {
		lines = append(lines, "state      health   plugin                         instance       summary")
		for _, row := range rows {
			lines = append(lines, rowText(
				tuiColumn{value: row.State.String(), width: 10},
				tuiColumn{value: pluginHealthText(row), width: 8},
				tuiColumn{value: row.PluginID, width: 30},
				tuiColumn{value: row.InstanceKey, width: 14},
				tuiColumn{value: pluginSummaryText(row), width: 36},
			))
		}
	}

	lines = append(lines, "", "Controllers")
	controllerRows := sortedDevtoolTUIControllerRows(snapshot)
	if len(controllerRows) == 0 {
		lines = append(lines, "no controller activity")
	}
	if len(controllerRows) > 0 {
		lines = append(lines, "state      kind     controller                    summary")
		for _, row := range controllerRows {
			lines = append(lines, rowText(
				tuiColumn{value: row.State.String(), width: 10},
				tuiColumn{value: row.Kind, width: 8},
				tuiColumn{value: row.ControllerID, width: 30},
				tuiColumn{value: firstNonEmpty(row.Error, row.Summary), width: 36},
			))
		}
	}
	return buildDevtoolTUIPanel("Plugins / Controllers", lines, tui.WithFlexGrow(1))
}

func buildDevtoolTUIAttentionPane(snapshot *devtool_status.BldrDevtoolStatus) *tui.Element {
	lines := []string{}
	for _, row := range sortedDevtoolTUIAttentionRows(snapshot) {
		lines = append(lines, rowText(
			tuiColumn{value: row.Severity.String(), width: 7},
			tuiColumn{value: row.Source, width: 18},
			tuiColumn{value: firstNonEmpty(row.Message, row.Detail), width: 64},
		))
	}
	if command := snapshot.GetCommand(); command.Error != "" {
		lines = append(lines, rowText(
			tuiColumn{value: "error", width: 7},
			tuiColumn{value: "command", width: 18},
			tuiColumn{value: command.Error, width: 64},
		))
	}
	for _, row := range sortedDevtoolTUIManifestFetchRows(snapshot) {
		if row.Error != "" {
			lines = append(lines, rowText(
				tuiColumn{value: "error", width: 7},
				tuiColumn{value: attentionSourceText("fetch", row.ID), width: 18},
				tuiColumn{value: row.Error, width: 64},
			))
		}
	}
	for _, row := range sortedDevtoolTUIManifestBuildRows(snapshot) {
		if row.Error != "" {
			lines = append(lines, rowText(
				tuiColumn{value: "error", width: 7},
				tuiColumn{value: attentionSourceText("build", row.ID), width: 18},
				tuiColumn{value: row.Error, width: 64},
			))
		}
	}
	for _, row := range sortedDevtoolTUIPluginRows(snapshot) {
		if row.Error != "" {
			lines = append(lines, rowText(
				tuiColumn{value: "error", width: 7},
				tuiColumn{value: attentionSourceText("plugin", row.ID), width: 18},
				tuiColumn{value: row.Error, width: 64},
			))
		}
	}
	for _, row := range sortedDevtoolTUIControllerRows(snapshot) {
		if row.Error != "" {
			lines = append(lines, rowText(
				tuiColumn{value: "error", width: 7},
				tuiColumn{value: attentionSourceText("controller", row.ID), width: 18},
				tuiColumn{value: row.Error, width: 64},
			))
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "no recent errors or attention")
	}
	return buildDevtoolTUIPanel("Recent Errors / Attention", lines, tui.WithFlexGrow(1))
}

func buildDevtoolTUIFooter(snapshot *devtool_status.BldrDevtoolStatus) *tui.Element {
	logFile := snapshot.GetCommand().LogFile
	if logFile == "" {
		logFile = ".bldr/logs"
	}
	lines := []string{
		"q: quit   ctrl-c: stop   logs: " + compactDevtoolTUILogFile(logFile),
	}
	return buildDevtoolTUIPanel("Controls", lines, tui.WithHeightAuto())
}

func buildDevtoolTUIPanel(title string, lines []string, opts ...tui.Option) *tui.Element {
	panelOpts := []tui.Option{
		tui.WithBorder(tui.BorderSingle),
		tui.WithPaddingTRBL(0, 1, 0, 1),
		tui.WithOverflow(tui.OverflowHidden),
		tui.WithText(title + "\n" + strings.Join(lines, "\n")),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.White)),
	}
	panelOpts = append(panelOpts, opts...)
	return tui.New(panelOpts...)
}

func manifestFetchReadyText(row devtool_status.BldrDevtoolManifestFetchRow) string {
	if row.BlockedOnLocalBuild {
		return "local"
	}
	if row.ReadyRefCount > 0 {
		return "ready:" + strconv.Itoa(row.ReadyRefCount)
	}
	return "-"
}

func manifestFetchSummaryText(row devtool_status.BldrDevtoolManifestFetchRow) string {
	if row.Error != "" {
		return row.Error
	}
	if row.BlockedOnLocalBuild {
		return firstNonEmpty(row.Summary, "blocked on local build")
	}
	return row.Summary
}

func manifestBuildModeText(row devtool_status.BldrDevtoolManifestBuildRow) string {
	switch {
	case row.CacheHit:
		return "cache"
	case row.HotRebuild:
		return "hot"
	case row.FullRebuild:
		return "full"
	default:
		return "-"
	}
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

func watchedFileCountText(count int) string {
	if count == 0 {
		return "-"
	}
	return strconv.Itoa(count)
}

func pluginHealthSummaryText(rows []devtool_status.BldrDevtoolPluginRow) string {
	var requested, running, errored, unknown int
	for _, row := range rows {
		switch row.State {
		case devtool_status.BldrDevtoolPluginStateRequested:
			requested++
		case devtool_status.BldrDevtoolPluginStateRunning:
			running++
		case devtool_status.BldrDevtoolPluginStateErrored:
			errored++
		default:
			unknown++
		}
	}
	return "health: running=" + strconv.Itoa(running) +
		" requested=" + strconv.Itoa(requested) +
		" errored=" + strconv.Itoa(errored) +
		" unknown=" + strconv.Itoa(unknown)
}

func pluginHealthText(row devtool_status.BldrDevtoolPluginRow) string {
	switch row.State {
	case devtool_status.BldrDevtoolPluginStateRequested:
		return "waiting"
	case devtool_status.BldrDevtoolPluginStateRunning:
		return "healthy"
	case devtool_status.BldrDevtoolPluginStateErrored:
		return "error"
	default:
		return "unknown"
	}
}

func pluginSummaryText(row devtool_status.BldrDevtoolPluginRow) string {
	summary := firstNonEmpty(row.Error, row.Summary)
	if row.Error != "" && row.LastErrorAt != "" {
		return summary + " at " + row.LastErrorAt
	}
	return summary
}

func currentDevtoolTUIWorkText(snapshot *devtool_status.BldrDevtoolStatus) string {
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
	if len(entries) == 0 {
		return "no active work"
	}
	if len(entries) > 4 {
		entries = append(entries[:4], "+"+strconv.Itoa(len(entries)-4)+" more")
	}
	return strings.Join(entries, " | ")
}

func attentionSourceText(kind, id string) string {
	if id == "" {
		return kind
	}
	return kind + ":" + strings.TrimPrefix(id, kind+":")
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

type tuiColumn struct {
	value string
	width int
}

func rowText(columns ...tuiColumn) string {
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		parts = append(parts, padTUIText(column.value, column.width))
	}
	return strings.Join(parts, " ")
}

func padTUIText(value string, width int) string {
	value = shortTUIText(value, width)
	if width < 1 {
		return value
	}
	for len([]rune(value)) < width {
		value += " "
	}
	return value
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

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
