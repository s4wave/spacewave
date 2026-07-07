//go:build !js

package devtool

import (
	"strconv"
	"strings"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

const (
	devtoolTUIMinWidth     = 40
	devtoolTUINarrowWidth  = 72
	devtoolTUIMaxRows      = 6
	devtoolTUIEmptyMessage = "waiting for status"
)

type devtoolTUIRegion struct {
	Title string
	Lines []string
}

func renderDevtoolTUIDashboard(snapshot *devtool_status.BldrDevtoolStatus, width int) string {
	if width < devtoolTUIMinWidth {
		width = devtoolTUIMinWidth
	}
	if snapshot == nil {
		snapshot = devtool_status.EmptyBldrDevtoolStatus()
	}

	var out strings.Builder
	command := snapshot.GetCommand()
	title := "Bldr Devtool"
	if command.Name != "" {
		title += " - " + command.Name
	}
	if command.State != devtool_status.BldrDevtoolCommandStateUnknown {
		title += " [" + command.State.String() + "]"
	}
	writeDashboardLine(&out, width, title)
	writeDashboardLine(&out, width, "ctrl-c cancel | o open browser | logs stay in .bldr/logs")
	out.WriteByte('\n')

	for idx, region := range buildDevtoolTUIRegions(snapshot, width) {
		if idx != 0 {
			out.WriteByte('\n')
		}
		writeDashboardLine(&out, width, "["+region.Title+"]")
		if len(region.Lines) == 0 {
			writeDashboardLine(&out, width, "  "+devtoolTUIEmptyMessage)
			continue
		}
		for _, line := range region.Lines {
			writeDashboardLine(&out, width, "  "+line)
		}
	}
	return out.String()
}

func buildDevtoolTUIRegions(snapshot *devtool_status.BldrDevtoolStatus, width int) []devtoolTUIRegion {
	return []devtoolTUIRegion{
		{Title: "command", Lines: commandRegionLines(snapshot.GetCommand(), width)},
		{Title: "manifest fetch", Lines: manifestFetchRegionLines(snapshot.GetManifestFetchRows(), width)},
		{Title: "manifest builds", Lines: manifestBuildRegionLines(snapshot.GetManifestBuildRows(), width)},
		{Title: "plugins", Lines: pluginRegionLines(snapshot.GetPluginRows(), width)},
		{Title: "controllers", Lines: controllerRegionLines(snapshot.GetControllerRows(), width)},
		{Title: "recent errors", Lines: recentErrorRegionLines(snapshot)},
	}
}

func commandRegionLines(command devtool_status.BldrDevtoolCommandStatus, width int) []string {
	lines := make([]string, 0, 4)
	name := command.Name
	if name == "" {
		name = "command"
	}
	state := command.State.String()
	lines = append(lines, name+" "+state)
	if command.Summary != "" {
		lines = append(lines, command.Summary)
	}
	if command.LogFile != "" {
		lines = append(lines, "log "+displayLogPath(command.LogFile))
	}
	if command.Error != "" {
		lines = append(lines, "error "+command.Error)
	}
	return compactRegionLines(lines, width)
}

func manifestFetchRegionLines(rows []devtool_status.BldrDevtoolManifestFetchRow, width int) []string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		detail := row.Summary
		if detail == "" && row.ReadyRefCount != 0 {
			detail = strconv.Itoa(row.ReadyRefCount) + " ready refs"
		}
		if detail == "" && row.LocalBuildIDs != "" {
			detail = "blocked on " + row.LocalBuildIDs
		}
		if row.Error != "" {
			detail = row.Error
		}
		lines = append(lines, statusRow(
			width,
			row.State.String(),
			row.ManifestID,
			row.PlatformID,
			row.BuildType,
			row.RemoteID,
			detail,
		))
	}
	return compactRegionLines(lines, width)
}

func manifestBuildRegionLines(rows []devtool_status.BldrDevtoolManifestBuildRow, width int) []string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		detail := row.Summary
		if detail == "" && row.DependencyRebuildReason != "" {
			detail = row.DependencyRebuildReason
		}
		if detail == "" && row.WatchedFileCount != 0 {
			detail = strconv.Itoa(row.WatchedFileCount) + " watched files"
		}
		if row.CacheHit {
			detail = appendSummary(detail, "cache hit")
		}
		if row.HotRebuild {
			detail = appendSummary(detail, "hot rebuild")
		}
		if row.FullRebuild {
			detail = appendSummary(detail, "full rebuild")
		}
		if row.Error != "" {
			detail = row.Error
		}
		lines = append(lines, statusRow(
			width,
			row.State.String(),
			row.ManifestID,
			row.PlatformID,
			row.BuildType,
			row.RemoteID,
			detail,
		))
	}
	return compactRegionLines(lines, width)
}

func pluginRegionLines(rows []devtool_status.BldrDevtoolPluginRow, width int) []string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		detail := row.Summary
		if row.InstanceKey != "" {
			detail = appendSummary(detail, row.InstanceKey)
		}
		if row.LastErrorAt != "" {
			detail = appendSummary(detail, row.LastErrorAt)
		}
		if row.Error != "" {
			detail = row.Error
		}
		lines = append(lines, statusRow(width, row.State.String(), row.PluginID, "", "", "", detail))
	}
	return compactRegionLines(lines, width)
}

func controllerRegionLines(rows []devtool_status.BldrDevtoolControllerRow, width int) []string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		detail := row.Summary
		if row.Kind != "" {
			detail = appendSummary(detail, row.Kind)
		}
		if row.Error != "" {
			detail = row.Error
		}
		lines = append(lines, statusRow(width, row.State.String(), row.ControllerID, "", "", "", detail))
	}
	return compactRegionLines(lines, width)
}

func recentErrorRegionLines(snapshot *devtool_status.BldrDevtoolStatus) []string {
	lines := make([]string, 0)
	command := snapshot.GetCommand()
	if command.Error != "" {
		lines = append(lines, "command: "+command.Error)
	}
	for _, row := range snapshot.GetManifestFetchRows() {
		if row.Error != "" {
			lines = append(lines, row.ManifestID+": "+row.Error)
		}
	}
	for _, row := range snapshot.GetManifestBuildRows() {
		if row.Error != "" {
			lines = append(lines, row.ManifestID+": "+row.Error)
		}
	}
	for _, row := range snapshot.GetPluginRows() {
		if row.Error != "" {
			lines = append(lines, row.PluginID+": "+row.Error)
		}
	}
	for _, row := range snapshot.GetControllerRows() {
		if row.Error != "" {
			lines = append(lines, row.ControllerID+": "+row.Error)
		}
	}
	for _, row := range snapshot.GetAttentionRows() {
		source := row.Source
		if source == "" {
			source = row.Severity.String()
		}
		message := row.Message
		if row.Detail != "" {
			message = appendSummary(message, row.Detail)
		}
		lines = append(lines, source+": "+message)
	}
	return compactRegionLines(lines, devtoolTUINarrowWidth)
}

func statusRow(width int, state, primary, platform, buildType, remote, detail string) string {
	if primary == "" {
		primary = "-"
	}
	parts := []string{state, primary}
	if platform != "" {
		parts = append(parts, platform)
	}
	if buildType != "" {
		parts = append(parts, buildType)
	}
	if remote != "" {
		parts = append(parts, remote)
	}
	if detail != "" {
		parts = append(parts, detail)
	}
	if width < devtoolTUINarrowWidth {
		return strings.Join(parts, " ")
	}
	return padRight(state, 10) + " " + strings.Join(parts[1:], "  ")
}

func compactRegionLines(lines []string, width int) []string {
	if len(lines) == 0 {
		return nil
	}
	if len(lines) > devtoolTUIMaxRows {
		remaining := len(lines) - devtoolTUIMaxRows + 1
		lines = append(lines[:devtoolTUIMaxRows-1], strconv.Itoa(remaining)+" more rows")
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, truncateDisplay(line, width-2))
	}
	return out
}

func writeDashboardLine(out *strings.Builder, width int, line string) {
	out.WriteString(truncateDisplay(line, width))
	out.WriteByte('\n')
}

func truncateDisplay(value string, width int) string {
	if width <= 0 || len(value) <= width {
		return value
	}
	if width == 1 {
		return value[:1]
	}
	return value[:width-1] + "…"
}

func padRight(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
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
