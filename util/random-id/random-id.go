package random_id

import (
	"github.com/aperturerobotics/bifrost/util/randstring"
)

// RandomIdentifier generates a random string identifier.
func RandomIdentifier() string {
	return randstring.RandString(nil, 8)
}
