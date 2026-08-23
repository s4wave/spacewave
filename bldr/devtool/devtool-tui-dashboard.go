//go:build !js

package devtool

import (
	"slices"
	"strconv"
	"strings"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

// The devtool dashboard is designed to be read in a glance. Sections are
// ordered by what a developer needs first, and failures are surfaced with
// their full error text near the top rather than truncated at the bottom.
//
// Layout (width 100, a failing build):
//
//	⟳ Bldr Devtool · start web · RUNNING
//	  web runtime active on 127.0.0.1:8080
//
//	SERVING
//	  ➜ http://127.0.0.1:8080          press o to open browser
//
//	FAILURES · 2
//	  ✗ build spacewave-app · web/js/wasm dev
//	      ./src/app/main.go:42:13: undefined: RenderRoot
//	      (did you mean RenderRootView?)
//	  ✗ plugin goscript-web
//	      worker exited: exit status 2
//
//	TARGETS · 3
//	  ✗ spacewave-app    web/js/wasm dev   build failed
//	  ⟳ spacewave-core   web/js/wasm dev   compiling · hot rebuild · 214 files
//	  ✓ spacewave-web    web/js/wasm dev   ready · 5 refs
//
//	RUNTIME
//	  plugins      2 running · 1 errored
//	  controllers  2 running · 1 idle
//
//	ctrl-c quit · o open browser · logs .bldr/logs/

const (
	devtoolTUIMinWidth      = 40
	devtoolTUINarrowWidth   = 64
	devtoolTUIMaxTargetRows = 12
	devtoolTUIMaxErrorLines = 6
	devtoolTUIStateWidth    = 6
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
	if width < devtoolTUIMinWidth {
		width = devtoolTUIMinWidth
	}
	if snapshot == nil {
		snapshot = devtool_status.EmptyBldrDevtoolStatus()
	}
	th := tuiTheme{color: color}

	sections := [][]string{
		headerSection(th, snapshot, width),
		servingSection(th, snapshot, servingURL, width),
		failureSection(th, snapshot, width),
		targetSection(th, snapshot, width),
		runtimeSection(th, snapshot, width),
		footerSection(th, width),
	}

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
	command := snapshot.GetCommand()
	kind := commandStatusKind(command.State)

	title := "Bldr Devtool"
	if command.Name != "" {
		title += " · " + command.Name
	}
	state := strings.ToUpper(command.State.String())
	plain := kind.glyph() + " " + title + " · " + state

	var line string
	if visibleWidth(plain) <= width {
		line = th.kind(kind, kind.glyph()) + " " +
			th.paint(ansiBold, title) + " · " + th.kind(kind, state)
	} else {
		line = th.kind(kind, kind.glyph()) + " " + fit(title+" · "+state, width-2)
	}

	lines := []string{line}
	if command.Summary != "" {
		lines = append(lines, th.paint(ansiDim, "  "+fit(command.Summary, width-2)))
	}
	return lines
}

// servingSection renders the address the devtool serves, when it serves one.
func servingSection(
	th tuiTheme,
	snapshot *devtool_status.BldrDevtoolStatus,
	servingURL string,
	width int,
) []string {
	project := snapshot.GetProject()
	if servingURL == "" && project.WebStartupPath == "" {
		return nil
	}
	lines := []string{sectionTitle(th, "SERVING", width)}
	if servingURL != "" {
		hint := "press o to open browser"
		url := th.paint(ansiCyan+ansiBold, servingURL)
		body := "  ➜ " + url
		if visibleWidth("  ➜ "+servingURL) < width-len(hint)-3 {
			pad := width - visibleWidth("  ➜ "+servingURL) - len(hint)
			body += strings.Repeat(" ", pad) + th.paint(ansiDim, hint)
		}
		lines = append(lines, body)
	}
	if project.WebStartupPath != "" {
		lines = append(lines, th.paint(ansiDim, "  entry "+fit(project.WebStartupPath, width-9)))
	}
	return lines
}

// tuiFailure is one problem surfaced with enough context to act on it.
type tuiFailure struct {
	where   string
	message string
	logPath string
	kind    tuiStatusKind
}

// failureSection lists every failing surface with its full error text wrapped
// on screen, so the developer sees what failed and where without opening logs.
func failureSection(th tuiTheme, snapshot *devtool_status.BldrDevtoolStatus, width int) []string {
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

func collectFailures(snapshot *devtool_status.BldrDevtoolStatus) []tuiFailure {
	var failures []tuiFailure
	command := snapshot.GetCommand()
	if command.Error != "" {
		failures = append(failures, tuiFailure{
			where:   "command " + commandName(command),
			message: command.Error,
			logPath: command.LogFile,
			kind:    tuiStatusError,
		})
	}
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
	manifest string
	platform string
	buildKit string
	detail   string
	kind     tuiStatusKind
}

// targetSection renders the unified per-target build table, active work first.
func targetSection(th tuiTheme, snapshot *devtool_status.BldrDevtoolStatus, width int) []string {
	targets := collectTargets(snapshot)
	if len(targets) == 0 {
		return nil
	}
	slices.SortStableFunc(targets, func(a, b tuiTarget) int {
		return a.kind.rank() - b.kind.rank()
	})

	lines := []string{sectionTitle(th, "TARGETS · "+strconv.Itoa(len(targets)), width)}
	nameWidth := targetNameWidth(targets)
	shown := targets
	if len(shown) > devtoolTUIMaxTargetRows {
		shown = shown[:devtoolTUIMaxTargetRows]
	}
	for _, target := range shown {
		lines = append(lines, targetLine(th, target, nameWidth, width))
	}
	if hidden := len(targets) - len(shown); hidden > 0 {
		lines = append(lines, th.paint(ansiDim, "  … "+strconv.Itoa(hidden)+" more targets"))
	}
	return lines
}

func collectTargets(snapshot *devtool_status.BldrDevtoolStatus) []tuiTarget {
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

func buildDetail(row devtool_status.BldrDevtoolManifestBuildRow) string {
	if row.State == devtool_status.BldrDevtoolManifestStateError {
		return "build failed"
	}
	detail := row.Summary
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
	if row.WatchedFileCount != 0 {
		detail = appendDetail(detail, strconv.Itoa(row.WatchedFileCount)+" files")
	}
	if detail == "" {
		detail = row.State.String()
	}
	return detail
}

func fetchDetail(row devtool_status.BldrDevtoolManifestFetchRow) string {
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

func targetLine(th tuiTheme, target tuiTarget, nameWidth, width int) string {
	glyph := th.kind(target.kind, target.kind.glyph())
	name := padRight(target.manifest, nameWidth)
	platform := strings.TrimSpace(target.platform + " " + target.buildKit)

	rest := name
	if width >= devtoolTUINarrowWidth {
		rest += "  " + padRight(platform, 16)
	} else if platform != "" {
		rest += "  " + platform
	}
	if target.detail != "" {
		rest += "  " + target.detail
	}
	detailKind := ansiDim
	if target.kind == tuiStatusError {
		detailKind = ""
	}
	return "  " + glyph + " " + th.paint(detailKind, fit(rest, width-4))
}

// runtimeSection collapses plugin and controller detail into a scannable count
// summary; individual failures already appear in the failures section.
func runtimeSection(th tuiTheme, snapshot *devtool_status.BldrDevtoolStatus, width int) []string {
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
func footerSection(th tuiTheme, width int) []string {
	return []string{th.paint(ansiDim, fit("ctrl-c quit · o open browser · logs .bldr/logs/", width))}
}

func sectionTitle(th tuiTheme, title string, width int) string {
	return th.paint(ansiBold, fit(title, width))
}

func countSummary(counts map[tuiStatusKind]int) string {
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

func targetNameWidth(targets []tuiTarget) int {
	width := 8
	for _, target := range targets {
		if n := len(target.manifest); n > width {
			width = n
		}
	}
	if width > 24 {
		width = 24
	}
	return width
}

func targetLabel(manifest, platform, buildType string) string {
	label := manifestName(manifest)
	tail := strings.TrimSpace(platform + " " + buildType)
	if tail != "" {
		label += " · " + tail
	}
	return label
}

func manifestName(id string) string {
	if id == "" {
		return "-"
	}
	return id
}

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

func fit(value string, width int) string {
	return truncateDisplay(value, width)
}

func truncateDisplay(value string, width int) string {
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

func padRight(value string, width int) string {
	pad := width - visibleWidth(value)
	if pad <= 0 {
		return value
	}
	return value + strings.Repeat(" ", pad)
}

func appendDetail(detail, next string) string {
	if next == "" {
		return detail
	}
	if detail == "" {
		return next
	}
	return detail + " · " + next
}

func appendSummary(summary, next string) string {
	if next == "" {
		return summary
	}
	if summary == "" {
		return next
	}
	return summary + "; " + next
}

func displayLogPath(path string) string {
	idx := strings.Index(path, ".bldr/logs/")
	if idx >= 0 {
		return path[idx:]
	}
	return path
}
