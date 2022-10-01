package plugin_compiler

import (
	"io"
	"os"
	"path/filepath"
)

// copyFileFromTo copies a file from a path to another path
func copyFileFromTo(src, dest string) error {
	ofile, oerr := os.Open(src)
	if oerr != nil {
		return oerr
	}
	defer ofile.Close()

	outFileFd, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer outFileFd.Close()

	_, err = io.Copy(outFileFd, ofile)
	if err != nil {
		return err
	}
	err = outFileFd.Sync()
	return err
}

// filepathHasPrefix checks if a file path has a prefix.
//
//nolint:staticcheck // filepath.HasPrefix is deprecated but OK in this use case
func filepathHasPrefix(p1, p2 string) bool {
	return filepath.HasPrefix(p1, p2)
}
