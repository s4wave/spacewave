//go:build js && !tinygo

package opfs

import (
	"syscall/js"

	"github.com/pkg/errors"
)

func getRootWithTinyGoImport() (js.Value, error) {
	return js.Undefined(), errors.New("tinygo OPFS import helper unavailable")
}

func getDirectoryWithTinyGoImport(_ js.Value, _ string, _ bool) (js.Value, error) {
	return js.Undefined(), errors.New("tinygo OPFS import helper unavailable")
}

func openAsyncFileWithTinyGoImport(_ js.Value, _ string, _ bool) (*AsyncFile, error) {
	return nil, errors.New("tinygo OPFS import helper unavailable")
}

func fileExistsWithTinyGoImport(_ js.Value, _ string) (bool, error) {
	return false, errors.New("tinygo OPFS import helper unavailable")
}

func deleteEntryWithTinyGoImport(_ js.Value, _ string, _ bool) error {
	return errors.New("tinygo OPFS import helper unavailable")
}

func yieldMicrotaskWithTinyGoImport() error {
	return errors.New("tinygo OPFS import helper unavailable")
}

func (f *AsyncFile) sizeWithTinyGoImport() (int64, error) {
	return 0, errors.New("tinygo OPFS import helper unavailable")
}

func (f *AsyncFile) truncateWithTinyGoImport(_ int64) error {
	return errors.New("tinygo OPFS import helper unavailable")
}

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
