package trace

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

// WriteTree writes the critical path tree to w.
func WriteTree(w io.Writer, root *TaskNode) {
	if root == nil {
		fmt.Fprintln(w, "(no complete root tasks found)")
		return
	}
	writeNode(w, root, "", true, true)
}

// writeNode recursively renders a task node as an ASCII tree.
func writeNode(w io.Writer, node *TaskNode, prefix string, isLast bool, isRoot bool) {
	// Connector for this line.
	var connector string
	if isRoot {
		connector = ""
	} else if isLast {
		connector = "'- "
	} else {
		connector = "|- "
	}

	// Format: name  duration  (pct%)  [xN]
	line := prefix + connector + node.Name
	durStr := fmtDur(node.Duration)
	if node.ParentPct > 0 {
		line = padTo(line, 60) + fmt.Sprintf("%8s  (%4.0f%%)", durStr, node.ParentPct)
	} else {
		line = padTo(line, 60) + fmt.Sprintf("%8s", durStr)
	}
	fmt.Fprintln(w, line)

	// Child prefix for subsequent lines.
	childPrefix := prefix
	if !isRoot {
		if isLast {
			childPrefix += "   "
		} else {
			childPrefix += "|  "
		}
	}

	// Aggregate children of the same type.
	children := aggregateChildren(node)

	// Calculate untraced time.
	var childTotal time.Duration
	for _, c := range children {
		childTotal += c.Duration
	}
	untraced := node.Duration - childTotal

	// Render each child, marking the last line.
	for i, child := range children {
		last := i == len(children)-1 && (untraced <= 0 || untraced < node.Duration/100)
		writeAggregatedNode(w, child, childPrefix, last, node)
	}

	// Show untraced time if > 1% of parent.
	if untraced > 0 && node.Duration > 0 && untraced >= node.Duration/100 {
		pct := float64(untraced) / float64(node.Duration) * 100
		line := childPrefix + "'- (untraced)"
		line = padTo(line, 60) + fmt.Sprintf("%8s  (%4.0f%%)", fmtDur(untraced), pct)
		fmt.Fprintln(w, line)
	}
}

// aggregatedChild groups same-type children.
type aggregatedChild struct {
	Name     string
	Duration time.Duration
	Count    int
	// Node is set when count == 1 (single instance).
	Node *TaskNode
	// Min holds stats for aggregated groups along with Max, P50 and P99.
	Min time.Duration
	Max time.Duration
	P50 time.Duration
	P99 time.Duration
}

// aggregateChildren groups children of the same type when count > threshold.
func aggregateChildren(parent *TaskNode) []*aggregatedChild {
	// A node without children has nothing to group.
	if len(parent.Children) == 0 {
		return nil
	}

	// Group by name, preserving first-seen order.
	type group struct {
		name      string
		total     time.Duration
		count     int
		durations []time.Duration
		nodes     []*TaskNode
	}
	seen := make(map[string]*group)
	var order []string
	for _, c := range parent.Children {
		// Strip common prefix with parent for display.
		name := stripPrefix(c.Name, parent.Name)
		g, ok := seen[name]
		if !ok {
			g = &group{name: name}
			seen[name] = g
			order = append(order, name)
		}
		g.count++
		g.total += c.Duration
		g.durations = append(g.durations, c.Duration)
		g.nodes = append(g.nodes, c)
	}

	// Build one entry per group in first-seen order.
	var result []*aggregatedChild
	for _, name := range order {
		g := seen[name]
		ac := &aggregatedChild{
			Name:     name,
			Duration: g.total,
			Count:    g.count,
		}
		if g.count == 1 {
			// Use original node for recursive rendering.
			ac.Node = g.nodes[0]
			ac.Node.Name = name // use stripped name
		} else {
			// Compute aggregate stats.
			sorted := slices.Sorted(slices.Values(g.durations))
			ac.Min = sorted[0]
			ac.Max = sorted[len(sorted)-1]
			ac.P50 = percentile(sorted, 0.50)
			ac.P99 = percentile(sorted, 0.99)
		}
		result = append(result, ac)
	}
	return result
}

// writeAggregatedNode renders an aggregated child.
func writeAggregatedNode(w io.Writer, ac *aggregatedChild, prefix string, isLast bool, parent *TaskNode) {
	// Choose the connector for this line.
	connector := "|- "
	if isLast {
		connector = "'- "
	}

	// Compute the share of the parent.
	pct := float64(ac.Duration) / float64(parent.Duration) * 100

	// A single instance renders with its own children.
	if ac.Count == 1 && ac.Node != nil {
		// Single instance: render recursively with children.
		ac.Node.ParentPct = pct
		writeNode(w, ac.Node, prefix, isLast, false)
		return
	}

	// Aggregated line.
	line := prefix + connector + ac.Name
	line = padTo(line, 60) + fmt.Sprintf("%8s  (%4.0f%%)  x%d", fmtDur(ac.Duration), pct, ac.Count)
	fmt.Fprintln(w, line)

	// Inline stats for aggregated groups.
	if ac.Count > 1 {
		childPrefix := prefix
		if isLast {
			childPrefix += "   "
		} else {
			childPrefix += "|  "
		}
		stats := fmt.Sprintf("%s    [p50=%s p99=%s max=%s]",
			childPrefix, fmtDur(ac.P50), fmtDur(ac.P99), fmtDur(ac.Max))
		fmt.Fprintln(w, stats)
	}
}

// stripPrefix removes the common parent prefix from a child task name.
// e.g. "alpha/so-engine/process-apply-tx-op/commit" under parent
// "alpha/so-engine/process-apply-tx-op" becomes "commit".
func stripPrefix(child, parent string) string {
	if strings.HasPrefix(child, parent+"/") {
		stripped := child[len(parent)+1:]
		if stripped != "" {
			return stripped
		}
	}
	return child
}

// padTo pads a string to at least n characters.
func padTo(s string, n int) string {
	if len(s) >= n {
		return s + "  "
	}
	return s + strings.Repeat(" ", n-len(s))
}
