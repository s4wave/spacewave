//go:build !js && !wasip1

package world_recovery

import (
	"encoding/binary"
	"errors"
	"os"
	"reflect"
	"strconv"
)

// captureIdentity binds persistent inode/device identity and mutation metadata.
func captureIdentity(path string) ([]byte, error) {
	// Require a regular file with platform-provided stable physical identity.
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("capture must be a regular file")
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

	// Read metadata only; access time may change from ordinary OS read semantics.
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
