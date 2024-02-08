package npm

import (
	"encoding/json"
	"os"
)

// LoadPackageVersion loads the version of the given package from a specified package.json file.
// Returns the version if successful, otherwise an empty string and the error if any.
// If not found, returns "", nil
func LoadPackageVersion(filePath, packageName string) (string, error) {
	type pkgJSON struct {
		Dependencies map[string]string `json:"dependencies"`
	}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return "", err
	}

	var packageJSON pkgJSON
	err = json.Unmarshal(fileContent, &packageJSON)
	if err != nil {
		return "", err
	}

	version, found := packageJSON.Dependencies[packageName]
	if !found {
		return "", nil
	}

	return version, nil
}
