package reliteral

import (
	"regexp"
	"slices"
	"strings"
)

// GenerateRegex takes a list of string literals and produces a regex to match any of them.
func GenerateRegex(strs []string, anchors bool) string {
	// Escape any regex meta characters in the input strings.
	strs = slices.Clone(strs)
	for i, s := range strs {
		strs[i] = regexp.QuoteMeta(s)
	}

	// Join the escaped strings with a "|" to create a pattern that matches any of them.
	re := "(" + strings.Join(strs, "|") + ")"
	if anchors {
		re = "^" + re + "$"
	}
	return re
}
