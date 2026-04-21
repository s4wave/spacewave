package confparse

import "regexp"

// ParseRegexp parses a regular expression.
// If the field is empty, returns nil, nil.
func ParseRegexp(re string) (*regexp.Regexp, error) {
	if re == "" {
		return nil, nil
	}
	return regexp.Compile(re)
}
