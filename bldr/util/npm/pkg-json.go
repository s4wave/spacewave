package npm

import (
	"os"

	"github.com/aperturerobotics/fastjson"
)

// LoadPackageVersion loads the version of the given package from a specified package.json file.
// Returns the version if successful, otherwise an empty string and the error if any.
// If not found, returns "", nil
func LoadPackageVersion(filePath, packageName string) (string, error) {
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return "", err
	}

	var p fastjson.Parser
	v, err := p.ParseBytes(fileContent)
	if err != nil {
		return "", err
	}

	version := string(v.GetStringBytes("dependencies", packageName))
	if version == "" {
		return "", nil
	}

	return version, nil
}
