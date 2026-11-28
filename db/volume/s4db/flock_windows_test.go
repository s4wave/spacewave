package volume_s4db_test

import (
	"errors"
	"os"
)

// boltLocked is unsupported on Windows, which refuses bolt conversion.
func boltLocked(*os.File) (bool, error) {
	return false, errors.New("flock is unsupported on windows")
}
