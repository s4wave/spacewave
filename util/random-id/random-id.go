package random_id

import (
	"strings"

	"github.com/aperturerobotics/bifrost/util/randstring"
)

// RandomIdentifier generates a random string identifier.
func RandomIdentifier() string {
	return strings.ToLower(randstring.RandString(nil, 8))
}
