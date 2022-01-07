package blockenc

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/pkg/errors"
)

func TestBlockEnc(t *testing.T) {
	var pass [32]byte
	randomize := func(dat []byte) {
		if _, err := rand.Read(dat); err != nil {
			t.Fatal(err.Error())
		}
	}
	randomize(pass[:])
	alloc, relBuf := NewPoolAlloc()
	for i := BlockEnc_BlockEnc_XCHACHA20_POLY1305; i <= BlockEnc_BlockEnc_MAX; i++ {
		if err := func() error {
			c, err := BuildBlockEnc(i, pass[:])
			if err != nil {
				return err
			}
			data := alloc(128)
			randomize(data[:])
			dataBefore := make([]byte, len(data))
			copy(dataBefore, data[:])
			out, err := c.Encrypt(alloc, data[:])
			relBuf(data)
			if err != nil {
				return err
			}
			if bytes.Equal(out[:], dataBefore) {
				return errors.New("data was identical after encrypt")
			}
			data, err = c.Decrypt(alloc, out)
			if err != nil {
				return err
			}
			if !bytes.Equal(data[:], dataBefore) {
				return errors.Errorf("data was not identical after decrypt: %v != expected %v", data, dataBefore)
			}
			return nil
		}(); err != nil {
			t.Fatalf("block enc %v: %v", i.String(), err)
		}
	}
}
