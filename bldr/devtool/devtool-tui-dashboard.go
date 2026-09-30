//go:build !js

package devtool

import (
	"slices"
	"strconv"
	"strings"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

const (
	// devtoolTUIMinWidth is the fallback width when the terminal size is unknown.
	devtoolTUIMinWidth = 40
	// devtoolTUINarrowWidth separates stacked target rows from inline rows.
	devtoolTUINarrowWidth = 64
	// devtoolTUIMaxTargetRows bounds the target list while retaining urgent work first.
	devtoolTUIMaxTargetRows = 12
	// devtoolTUIMaxErrorLines bounds the visible explanation for a single failure.
	devtoolTUIMaxErrorLines = 6
)

// renderDevtoolTUIDashboard renders the full devtool status screen for the
// given terminal width. servingURL is the address the devtool serves, or empty.
// color enables ANSI styling; tests render with color disabled.
func renderDevtoolTUIDashboard(
	snapshot *devtool_status.BldrDevtoolStatus,
	servingURL string,
	width int,
	color bool,
) string {
	// Resolve absent inputs without widening a narrow terminal.
	if width <= 0 {
		width = devtoolTUIMinWidth
	}
	if snapshot == nil {
		snapshot = devtool_status.EmptyBldrDevtoolStatus()
	}
	th := tuiTheme{color: color}

	// Place failures before build progress and supporting runtime counts.
	sections := [][]string{
		headerSection(th, snapshot, width),
		servingSection(th, servingURL, width),
		failureSection(th, snapshot, width),
		targetSection(th, snapshot, width),
		runtimeSection(th, snapshot, width),
		footerSection(th, servingURL, width),
	}

	// Separate only sections that have content.
	var out strings.Builder
	first := true
	for _, section := range sections {
		if len(section) == 0 {
			continue
		}
		if !first {
			out.WriteByte('\n')
		}
		first = false
		for _, line := range section {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// headerSection renders the title line with the command name and live state.
func headerSection(th tuiTheme, snapshot *devtool_status.BldrDevtoolStatus, width int) []string {
	// Reserve the command state before fitting the title.
	command := snapshot.GetCommand()
	kind := commandStatusKind(command.State)
	title := "Bldr"
	if command.Name != "" {
		title += " · " + command.Name
	}
	badge := kind.glyph() + " " + strings.ToUpper(command.State.String())
	titleWidth := width - visibleWidth(badge) - 2
	lines := []string{th.paint(ansiCyan+ansiBold, padRight(fit(title, titleWidth), titleWidth)) + "  " + th.kind(kind, badge)}
	if titleWidth < 4 {
		lines = []string{th.paint(ansiBold, fit(title, width)), th.kind(kind, fit(badge, width))}
	}

	// Keep startup context with the command without implying a server is ready.
	if command.Summary != "" {
		lines = append(lines, th.paint(ansiDim, fit("  "+command.Summary, width)))
	}
	if entry := snapshot.GetProject().WebStartupPath; entry != "" {
		lines = append(lines, th.paint(ansiDim, fit("  entry "+entry, width)))
	}
	return lines
}

// servingSection renders the address the devtool serves, when it serves one.
func servingSection(th tuiTheme, servingURL string, width int) []string {
	if servingURL == "" {
		return nil
	}
	return []string{
		sectionTitle(th, "SERVING", width),
		th.paint(ansiCyan+ansiBold, fit("  ➜ "+servingURL, width)),
	}
}

// tuiFailure is one problem surfaced with enough context to act on it.
type tuiFailure struct {
	// where identifies the failed operation.
	where string
	// message contains the actionable error text.
	message string
	// logPath points to the full command log.
	logPath string
	// kind determines the failure marker and color.
	kind tuiStatusKind
}

// failureSection lists every failing surface with its full error text wrapped
// on screen, so the developer sees what failed and where without opening logs.
func failureSection(th tuiTheme, snapshot *devtool_status.BldrDevtoolStatus, width int) []string {
	// Render each collected failure as a titled wrapped list.
	failures := collectFailures(snapshot)
	if len(failures) == 0 {
		return nil
	}
	lines := []string{sectionTitle(th, "FAILURES · "+strconv.Itoa(len(failures)), width)}
	for _, failure := range failures {
		glyph := th.kind(failure.kind, failure.kind.glyph())
		lines = append(lines, "  "+glyph+" "+fit(failure.where, width-4))
		for _, wrapped := range wrapText(failure.message, width-6) {
			lines = append(lines, "      "+wrapped)
		}
		if failure.logPath != "" {
			lines = append(lines, th.paint(ansiDim, "      log "+fit(displayLogPath(failure.logPath), width-10)))
		}
	}
	return lines
}

// collectFailures gathers actionable errors from the existing status snapshot.
func collectFailures(snapshot *devtool_status.BldrDevtoolStatus) []tuiFailure {
	// Collect failures from every status surface into one list.
	var failures []tuiFailure

	// Add the command error with its log path.
	command := snapshot.GetCommand()
	if command.Error != "" {
		failures = append(failures, tuiFailure{
			where:   "command " + commandName(command),
			message: command.Error,
			logPath: command.LogFile,
			kind:    tuiStatusError,
		})
	}

	// Add each failing manifest fetch and build row.
	for _, row := range snapshot.GetManifestFetchRows() {
		if row.Error != "" {
			failures = append(failures, tuiFailure{
				where:   "fetch " + targetLabel(row.ManifestID, row.PlatformID, row.BuildType),
				message: row.Error,
				kind:    tuiStatusError,
			})
		}
	}
	for _, row := range snapshot.GetManifestBuildRows() {
		if row.Error != "" {
			failures = append(failures, tuiFailure{
				where:   "build " + targetLabel(row.ManifestID, row.PlatformID, row.BuildType),
				message: row.Error,
				kind:    tuiStatusError,
			})
		}
	}

	// Add each failing plugin and controller row.
	for _, row := range snapshot.GetPluginRows() {
		if row.Error != "" {
			where := "plugin " + row.PluginID
			if row.InstanceKey != "" {
				where += " · " + row.InstanceKey
			}
			failures = append(failures, tuiFailure{where: where, message: row.Error, kind: tuiStatusError})
		}
	}
	for _, row := range snapshot.GetControllerRows() {
		if row.Error != "" {
			failures = append(failures, tuiFailure{
				where:   "controller " + row.ControllerID,
				message: row.Error,
				kind:    tuiStatusError,
			})
		}
	}

	// Add warning and error attention rows.
	for _, row := range snapshot.GetAttentionRows() {
		if row.Severity != devtool_status.BldrDevtoolAttentionSeverityWarning &&
			row.Severity != devtool_status.BldrDevtoolAttentionSeverityError {
			continue
		}
		source := row.Source
		if source == "" {
			source = row.Severity.String()
		}
		failures = append(failures, tuiFailure{
			where:   source,
			message: appendSummary(row.Message, row.Detail),
			kind:    attentionStatusKind(row.Severity),
		})
	}
	return failures
}

// tuiTarget is one build unit shown in the targets table, merging the fetch and
// build views into a single per-manifest line the developer thinks in terms of.
type tuiTarget struct {
	// manifest identifies the build unit.
	manifest string
	// platform identifies the runtime and architecture.
	platform string
	// buildKit identifies the development or release configuration.
	buildKit string
	// detail describes current work or availability.
	detail string
	// kind determines the target order, marker, and color.
	kind tuiStatusKind
}

// targetSection renders the unified per-target build table, active work first.
func targetSection(th tuiTheme, snapshot *devtool_status.BldrDevtoolStatus, width int) []string {
	// Sort urgent work before completed artifacts.
	targets := collectTargets(snapshot)
	if len(targets) == 0 {
		return nil
	}
	slices.SortStableFunc(targets, func(a, b tuiTarget) int { return a.kind.rank() - b.kind.rank() })

	// Show readiness and common build settings once for the whole table.
	ready := 0
	platform, buildKit := targets[0].platform, targets[0].buildKit
	common := true
	for _, target := range targets {
		if target.kind == tuiStatusReady {
			ready++
		}
		if target.platform != platform || target.buildKit != buildKit {
			common = false
		}
	}
	lines := []string{sectionTitle(th, "TARGETS · "+strconv.Itoa(ready)+"/"+strconv.Itoa(len(targets))+" ready", width)}
	if common && (platform != "" || buildKit != "") {
		lines = append(lines, th.paint(ansiDim, fit("  "+strings.TrimSpace(platform+" "+buildKit), width)))
	}

	// Bound the table while preserving each visible target's status.
	nameWidth := targetNameWidth(targets)
	shown := targets[:min(len(targets), devtoolTUIMaxTargetRows)]
	for _, target := range shown {
		if common {
			target.platform, target.buildKit = "", ""
		}
		lines = append(lines, targetLine(th, target, nameWidth, width))
	}
	if hidden := len(targets) - len(shown); hidden > 0 {
		lines = append(lines, th.paint(ansiDim, fit("  … "+strconv.Itoa(hidden)+" more targets", width)))
	}
	return lines
}

// collectTargets combines local builds and standalone fetched artifacts.
func collectTargets(snapshot *devtool_status.BldrDevtoolStatus) []tuiTarget {
	// Include local builders as one row per build unit.
	buildRows := snapshot.GetManifestBuildRows()
	targets := make([]tuiTarget, 0, len(buildRows))
	for _, row := range buildRows {
		targets = append(targets, tuiTarget{
			manifest: manifestName(row.ManifestID),
			platform: row.PlatformID,
			buildKit: row.BuildType,
			detail:   buildDetail(row),
			kind:     manifestStatusKind(row.State),
		})
	}

	// Fetch rows not backed by a local build are standalone remote/cache
	// artifacts; surface them so the target list is complete.
	for _, row := range snapshot.GetManifestFetchRows() {
		if row.LocalBuildIDs != "" {
			continue
		}
		targets = append(targets, tuiTarget{
			manifest: manifestName(row.ManifestID),
			platform: row.PlatformID,
			buildKit: row.BuildType,
			detail:   fetchDetail(row),
			kind:     manifestStatusKind(row.State),
		})
	}
	return targets
}

// buildDetail describes current build work without repeating strategy labels.
func buildDetail(row devtool_status.BldrDevtoolManifestBuildRow) string {
	// Map the finished states to fixed detail text.
	if row.State == devtool_status.BldrDevtoolManifestStateError {
		return "build failed"
	}
	if row.State == devtool_status.BldrDevtoolManifestStateReady {
		if row.CacheHit {
			return "ready · cache hit"
		}
		return "ready"
	}

	// Describe the current operation before its rebuild strategy.
	detail := row.Summary
	if detail == "full rebuild" || detail == "hot rebuild" {
		detail = "building · " + detail
	}
	if detail == "" && row.DependencyRebuildReason != "" {
		detail = row.DependencyRebuildReason
	}
	if row.CacheHit {
		detail = appendDetail(detail, "cache hit")
	}
	if row.HotRebuild {
		detail = appendDetail(detail, "hot rebuild")
	}
	if row.FullRebuild {
		detail = appendDetail(detail, "full rebuild")
	}
	if detail == "" {
		detail = row.State.String()
	}
	return detail
}

// fetchDetail describes artifact availability and unresolved local dependencies.
func fetchDetail(row devtool_status.BldrDevtoolManifestFetchRow) string {
	// Report a fetch failure before its availability details.
	if row.Error != "" {
		return "fetch failed"
	}
	detail := row.Summary
	if row.ReadyRefCount != 0 {
		detail = appendDetail(detail, strconv.Itoa(row.ReadyRefCount)+" refs")
	}
	if row.BlockedOnLocalBuild {
		detail = appendDetail(detail, "waiting on local build")
	}
	if detail == "" {
		detail = row.State.String()
	}
	return detail
}

// targetLine keeps the target name and current work readable at the given width.
func targetLine(th tuiTheme, target tuiTarget, nameWidth, width int) string {
	// Fit the name separately so a long identifier cannot hide progress.
	glyph := th.kind(target.kind, target.kind.glyph())
	nameWidth = min(nameWidth, max(8, (width-6)/2))
	name := th.paint(ansiBold, padRight(fit(target.manifest, nameWidth), nameWidth))
	line := "  " + glyph + " " + name + "  " + th.kind(target.kind, fit(target.detail, width-nameWidth-6))
	if width < devtoolTUINarrowWidth {
		line = "  " + glyph + " " + th.paint(ansiBold, fit(target.manifest, width-4)) +
			"\n    " + th.kind(target.kind, fit(target.detail, width-4))
	}

	// Mixed-platform builds keep their settings attached to each target.
	if platform := strings.TrimSpace(target.platform + " " + target.buildKit); platform != "" {
		line += "\n    " + th.paint(ansiDim, fit(platform, width-4))
	}
	return line
}

// runtimeSection collapses plugin and controller detail into a scannable count
// summary; individual failures already appear in the failures section.
func runtimeSection(th tuiTheme, snapshot *devtool_status.BldrDevtoolStatus, width int) []string {
	// Summarize the plugin and controller row counts under one section title.
	plugins := snapshot.GetPluginRows()
	controllers := snapshot.GetControllerRows()
	if len(plugins) == 0 && len(controllers) == 0 {
		return nil
	}
	lines := []string{sectionTitle(th, "RUNTIME", width)}
	if len(plugins) > 0 {
		counts := map[tuiStatusKind]int{}
		for _, row := range plugins {
			counts[pluginStatusKind(row.State)]++
		}
		lines = append(lines, th.paint(ansiDim, "  plugins      "+fit(countSummary(counts), width-15)))
	}
	if len(controllers) > 0 {
		counts := map[tuiStatusKind]int{}
		for _, row := range controllers {
			counts[controllerStatusKind(row.State)]++
		}
		lines = append(lines, th.paint(ansiDim, "  controllers  "+fit(countSummary(counts), width-15)))
	}
	return lines
}

// footerSection renders the keyboard and log hints.
func footerSection(th tuiTheme, servingURL string, width int) []string {
	keys := "ctrl-c quit"
	if servingURL != "" {
		keys = "o open browser · " + keys
	}
	return []string{th.paint(ansiDim, fit(keys, width)), th.paint(ansiDim, fit("logs .bldr/logs/", width))}
}

// sectionTitle separates sections with a dim rule sized to the terminal.
func sectionTitle(th tuiTheme, title string, width int) string {
	label := fit(title, width)
	rule := strings.Repeat("─", max(0, width-visibleWidth(label)-2))
	if rule == "" {
		return th.paint(ansiBold, label)
	}
	return th.paint(ansiBold, label) + th.paint(ansiDim, "  "+rule)
}

// countSummary formats runtime counts in a stable status order.
func countSummary(counts map[tuiStatusKind]int) string {
	// Build the summary parts in a stable status order.
	order := []struct {
		kind  tuiStatusKind
		label string
	}{
		{tuiStatusActive, "running"},
		{tuiStatusPending, "pending"},
		{tuiStatusReady, "ready"},
		{tuiStatusIdle, "idle"},
		{tuiStatusError, "errored"},
		{tuiStatusWarn, "warning"},
		{tuiStatusNeutral, "unknown"},
	}
	parts := make([]string, 0, len(order))
	for _, entry := range order {
		if n := counts[entry.kind]; n > 0 {
			parts = append(parts, strconv.Itoa(n)+" "+entry.label)
		}
	}
	if len(parts) == 0 {
		return "0"
	}
	return strings.Join(parts, " · ")
}

// targetNameWidth bounds the name column by the visible target identifiers.
func targetNameWidth(targets []tuiTarget) int {
	width := 8
	for _, target := range targets {
		if n := visibleWidth(target.manifest); n > width {
			width = n
		}
	}
	if width > 24 {
		width = 24
	}
	return width
}

// targetLabel identifies the manifest and build settings for a failure.
func targetLabel(manifest, platform, buildType string) string {
	label := manifestName(manifest)
	tail := strings.TrimSpace(platform + " " + buildType)
	if tail != "" {
		label += " · " + tail
	}
	return label
}

// manifestName supplies a visible placeholder for an unnamed manifest.
func manifestName(id string) string {
	if id == "" {
		return "-"
	}
	return id
}

// commandName supplies a label for failures without a command name.
func commandName(command devtool_status.BldrDevtoolCommandStatus) string {
	if command.Name == "" {
		return "command"
	}
	return command.Name
}

// wrapText word-wraps text to width, capping the number of lines so a single
// verbose error cannot flood the screen while still showing its substance. The
// final line absorbs any overflow, truncated to width, so no text is silently
// dropped without a trailing ellipsis.
func wrapText(text string, width int) []string {
	// Fit the width and bail out on empty text before wrapping words.
	if width < 8 {
		width = 8
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil
	}
	var lines []string
	current := ""
	for idx, field := range fields {
		if current == "" {
			current = field
		} else if visibleWidth(current)+1+visibleWidth(field) <= width {
			current += " " + field
		} else if len(lines) == devtoolTUIMaxErrorLines-1 {
			current = current + " " + strings.Join(fields[idx:], " ")
			break
		} else {
			lines = append(lines, current)
			current = field
		}
	}
	lines = append(lines, fit(current, width))
	return lines
}

// fit truncates text to the available display width.
func fit(value string, width int) string {
	return truncateDisplay(value, width)
}

// truncateDisplay marks text omitted at the right edge with an ellipsis.
func truncateDisplay(value string, width int) string {
	// Return short values and empty widths without truncation.
	if width <= 0 {
		return ""
	}
	if visibleWidth(value) <= width {
		return value
	}
	runes := []rune(value)
	if width == 1 {
		return string(runes[:1])
	}
	return string(runes[:width-1]) + "…"
}

// padRight fills the remainder of a display column with spaces.
func padRight(value string, width int) string {
	pad := width - visibleWidth(value)
	if pad <= 0 {
		return value
	}
	return value + strings.Repeat(" ", pad)
}

// appendDetail joins distinct status details with a middle dot.
func appendDetail(detail, next string) string {
	if next == "" || slices.Contains(strings.Split(detail, " · "), next) {
		return detail
	}
	if detail == "" {
		return next
	}
	return detail + " · " + next
}

// appendSummary joins a summary with its supporting explanation.
func appendSummary(summary, next string) string {
	if next == "" {
		return summary
	}
	if summary == "" {
		return next
	}
	return summary + "; " + next
}

// displayLogPath shortens project log paths while retaining other locations.
func displayLogPath(path string) string {
	idx := strings.Index(path, ".bldr/logs/")
	if idx >= 0 {
		return path[idx:]
	}
	return path
}
