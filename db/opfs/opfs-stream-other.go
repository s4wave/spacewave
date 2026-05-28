//go:build js && !tinygo

package opfs

import (
	"syscall/js"

	"github.com/pkg/errors"
)

func createWriteStreamWithTinyGoImport(js.Value, string) (*WriteStream, error) {
	return nil, errors.New("tinygo write stream import unavailable")
}

func (w *WriteStream) writeWithTinyGoImport([]byte) (int, error) {
	return 0, errors.New("tinygo write stream import unavailable")
}

func (w *WriteStream) closeWithTinyGoImport() error {
	return errors.New("tinygo write stream import unavailable")
}

func (w *WriteStream) abortWithTinyGoImport() error {
	return nil
}
