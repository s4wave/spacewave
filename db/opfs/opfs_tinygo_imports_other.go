//go:build js && !tinygo

package opfs

import (
	"syscall/js"

	"github.com/pkg/errors"
)

func (f *AsyncFile) readAtWithTinyGoImport(_ []byte, _ int64) (int, error) {
	return 0, errors.New("tinygo OPFS import helper unavailable")
}

func (f *AsyncFile) writeAtWithTinyGoImport(_ []byte, _ int64, _ bool) (int, error) {
	return 0, errors.New("tinygo OPFS import helper unavailable")
}

func writeFileWithTinyGoImport(_ js.Value, _ string, _ []byte) error {
	return errors.New("tinygo OPFS import helper unavailable")
}

func readFileWithTinyGoImport(_ js.Value, _ string) ([]byte, error) {
	return nil, errors.New("tinygo OPFS import helper unavailable")
}

func listDirectoryWithTinyGoImport(_ js.Value) ([]string, error) {
	return nil, errors.New("tinygo OPFS import helper unavailable")
}
