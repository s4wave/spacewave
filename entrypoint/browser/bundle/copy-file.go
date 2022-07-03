package entrypoint_browser_bundle

import (
	"io"
	"os"
)

// CopyFile copies the contents from src to dst.
func CopyFile(dst, src string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	if err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
	}
	return err
}
