package bloom

import "github.com/pkg/errors"

var ErrIncompatibleBloom = errors.New("incompatible bloom filters")

// ErrInconsistentBloom is returned for a bloom filter whose words hold fewer
// bits than its size.
var ErrInconsistentBloom = errors.New("bloom filter holds fewer bits than its size")
