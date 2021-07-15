package longestcommon

import "strings"

// TrimPrefix removes the longest common prefix from all provided strings
func TrimPrefix(strs ...string) {
	p := Prefix(strs...)
	if p == "" {
		return
	}
	for i, s := range strs {
		strs[i] = strings.TrimPrefix(s, p)
	}
}

// Prefix returns the longest common prefix of the provided strings
// https://leetcode.com/problems/longest-common-prefix/discuss/374737/golang-runtime-0ms-simple-solution
func Prefix(strs ...string) string {
	if len(strs) == 0 {
		return ""
	}
	// Find word with minimum length
	short := strs[0]
	for _, s := range strs {
		if len(short) >= len(s) {
			short = s
		}
	}
	prefx_array := []string{}
	prefix := ""
	old_prefix := ""
	for i := 0; i < len(short); i++ {
		prefx_array = append(prefx_array, string(short[i]))
		prefix = strings.Join(prefx_array, "")
		for _, s := range strs {
			if !strings.HasPrefix(s, prefix) {
				return old_prefix
			}
		}
		old_prefix = prefix
	}
	return prefix
}
