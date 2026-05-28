package segment

import (
	"encoding/binary"
	"hash/crc32"
	"io"

	"github.com/pkg/errors"
)

// VerifyChecksum verifies an SSTable footer checksum without loading the whole
// file into memory.
func VerifyChecksum(r io.ReaderAt, size int64) error {
	if size < HeaderSize+4 {
		return errors.New("file too small for SSTable")
	}

	contentSize := size - 4
	var footerBuf [4]byte
	if _, err := r.ReadAt(footerBuf[:], contentSize); err != nil {
		return errors.Wrap(err, "read footer")
	}
	expected := binary.BigEndian.Uint32(footerBuf[:])

	crc := crc32.NewIEEE()
	if _, err := io.CopyBuffer(crc, io.NewSectionReader(r, 0, contentSize), make([]byte, 64*1024)); err != nil {
		return errors.Wrap(err, "read content")
	}
	if actual := crc.Sum32(); expected != actual {
		return errors.Errorf("CRC32 mismatch: expected %08x, got %08x", expected, actual)
	}
	return nil
}
