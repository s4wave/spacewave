package volume

import "regexp"

// CheckIDMatchesList checks if the ID matches the list or regex.
// If the ID matches the regex OR matches the list, returns true.
func CheckIDMatchesList(id string, list []string, re *regexp.Regexp) bool {
	for _, val := range list {
		if val == id {
			return true
		}
	}
	if re != nil && re.MatchString(id) {
		return true
	}
	return false
}

// CheckIDMatchesAliases checks if the ID matches the value or any alias.
// Returns true if the target id was empty
func CheckIDMatchesAliases(targetVolID, volID string, aliases []string) bool {
	if targetVolID == "" {
		return true
	}
	if volID == targetVolID {
		return true
	}
	for _, aliasID := range aliases {
		if aliasID == targetVolID {
			return true
		}
	}
	return false
}
