package npm

import (
	"os"

	"github.com/aperturerobotics/fastjson"
)

// LoadPackageVersion loads the version of the given package from a specified package.json file.
// Returns the version if successful, otherwise an empty string and the error if any.
// If not found, returns "", nil
func LoadPackageVersion(filePath, packageName string) (string, error) {
	// Read package.json, treating an absent manifest as an absent dependency.
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return "", err
	}

	// Parse the package manifest for dependency lookup.
	var p fastjson.Parser
	v, err := p.ParseBytes(fileContent)
	if err != nil {
		return "", err
	}

	// Resolve the requested dependency version from the manifest.
	version := string(v.GetStringBytes("dependencies", packageName))
	if version == "" {
		return "", nil
	}

	return version, nil
}
