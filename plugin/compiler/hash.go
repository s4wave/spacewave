package plugin_compiler

import (
	"io"
	"os"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/zeebo/blake3"
)

// HashPluginForBuildID hashes a plugin to use for build id.
// hhBaseKey should be unique for the plugin.
// hhBaseKey should be 32 bytes long
func HashPluginForBuildID(hhBaseKey []byte, filePath string) (string, error) {
	// hash pass 1 to determine build prefix
	hasher, err := blake3.NewKeyed(hhBaseKey)
	if err != nil {
		return "", err
	}
	ofile, oerr := os.Open(filePath)
	if oerr != nil {
		return "", oerr
	}
	_, err = io.Copy(hasher, ofile)
	ofile.Close()
	if err != nil && err != io.EOF {
		return "", err
	}
	return b58.Encode(hasher.Sum(nil)), nil
}
