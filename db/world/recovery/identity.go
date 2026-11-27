//go:build !js && !wasip1

package world_recovery

import (
	"encoding/binary"
	"errors"
	"os"
	"reflect"
	"strconv"
)

// captureIdentity binds the capture file's and every declared pack
// directory's persistent identity and mutation metadata.
func captureIdentity(source *Source) ([]byte, error) {
	// Bind the capture file, then each declared pack directory.
	identity, err := statIdentity(source.GetPath(), false)
	if err != nil {
		return nil, err
	}
	dirs, err := packDirIdentity(source.GetPacks())
	if err != nil {
		return nil, err
	}
	return append(identity, dirs...), nil
}

// statIdentity binds persistent inode/device identity and mutation metadata
// of a regular file, or of a directory when dir is set.
func statIdentity(path string, dir bool) ([]byte, error) {
	// Require a regular file or directory with stable physical identity.
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if dir != info.IsDir() || (!dir && !info.Mode().IsRegular()) {
		return nil, errors.New("capture must be a regular file and pack sources directories")
	}
	value := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return nil, errors.New("capture physical identity is unsupported on this platform")
	}
	var identity []byte
	for _, field := range []string{"Dev", "Ino"} {
		number := value.FieldByName(field)
		if !number.IsValid() {
			return nil, errors.New("capture physical identity is unsupported on this platform")
		}
		if number.CanUint() {
			identity = strconv.AppendUint(identity, number.Uint(), 10)
		} else if number.CanInt() {
			identity = strconv.AppendInt(identity, number.Int(), 10)
		} else {
			return nil, errors.New("invalid capture physical identity")
		}
		identity = append(identity, 0)
	}

	// Read metadata only. Access time may change from ordinary OS read
	// semantics.
	identity = append(identity, strconv.AppendInt(nil, info.Size(), 10)...)
	identity = append(identity, 0)
	identity = append(identity, strconv.AppendInt(nil, info.ModTime().UnixNano(), 10)...)
	identity = append(identity, 0)
	identity = binary.BigEndian.AppendUint32(identity, uint32(info.Mode()))
	for _, field := range []string{"Ctim", "Ctimespec"} {
		stamp := value.FieldByName(field)
		if stamp.IsValid() && stamp.Kind() == reflect.Struct {
			identity = append(identity, strconv.AppendInt(nil, stamp.FieldByName("Sec").Int(), 10)...)
			identity = append(identity, 0)
			identity = append(identity, strconv.AppendInt(nil, stamp.FieldByName("Nsec").Int(), 10)...)
			identity = append(identity, 0)
			break
		}
	}
	return identity, nil
}

// ReadResult reads the durable scratch report without opening its source.
func ReadResult(scratch string) (*Result, error) {
	// Read one bounded persisted result through a read-only scratch handle.
	db, err := openScratch(scratch, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	result := &Result{}
	err = db.View(func(tx *scratchTx) error {
		state := tx.Bucket([]byte("state"))
		if state == nil || state.Get([]byte("result")) == nil {
			return errors.New("scratch has no scan result")
		}
		return result.UnmarshalVT(state.Get([]byte("result")))
	})
	return result, err
}
