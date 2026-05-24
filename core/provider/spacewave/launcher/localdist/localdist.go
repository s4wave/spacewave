package localdist

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// Filename is the package-shipped fallback dist config filename.
const Filename = "dist-config.packedmsg"

// Paths returns candidate package dist-config paths for exePath.
func Paths(exePath string) []string {
	exeDir := filepath.Dir(exePath)
	return []string{
		filepath.Join(exeDir, Filename),
		filepath.Join(exeDir, "..", "Resources", Filename),
	}
}

// Read reads the first available local dist config path.
func Read(paths []string) ([]byte, string, error) {
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, "", err
		}
		if len(data) == 0 {
			return nil, "", errors.Errorf("local dist config is empty: %s", p)
		}
		return data, p, nil
	}
	return nil, "", nil
}
