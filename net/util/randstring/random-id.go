package randstring

import "strings"

// RandomIdentifier generates a random string identifier.
func RandomIdentifier(idLen int) string {
	if idLen == 0 {
		idLen = 8
	}
	return strings.ToLower(RandString(nil, idLen))
}
