package order

import (
	"io"
	"os"

	"github.com/pkg/errors"
)

// EncodeAccessOrderRecord writes record as protobuf binary data.
func EncodeAccessOrderRecord(w io.Writer, record *AccessOrderRecord) error {
	// Require an access order record before encoding it.
	if record == nil {
		return errors.New("packfile/order: access order record is nil")
	}

	// Encode the access order record as protobuf data.
	dat, err := record.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal access order record")
	}

	// Write the encoded access order record to the destination.
	_, err = w.Write(dat)
	return errors.Wrap(err, "write access order record")
}

// DecodeAccessOrderRecord reads protobuf binary data into a record.
func DecodeAccessOrderRecord(r io.Reader) (*AccessOrderRecord, error) {
	// Read the encoded access order record from the source.
	dat, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Wrap(err, "read access order record")
	}

	// Decode the protobuf data into an access order record.
	record := &AccessOrderRecord{}
	if err := record.UnmarshalVT(dat); err != nil {
		return nil, errors.Wrap(err, "unmarshal access order record")
	}

	return record, nil
}

// WriteAccessOrderRecordFile writes record to path as protobuf binary data.
func WriteAccessOrderRecordFile(path string, record *AccessOrderRecord) error {
	file, err := os.Create(path)
	if err != nil {
		return errors.Wrap(err, "create access order record")
	}
	if err := EncodeAccessOrderRecord(file, record); err != nil {
		_ = file.Close()
		return err
	}
	return errors.Wrap(file.Close(), "close access order record")
}

// ReadAccessOrderRecordFile reads path as protobuf binary data.
func ReadAccessOrderRecordFile(path string) (*AccessOrderRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "open access order record")
	}
	defer file.Close()
	return DecodeAccessOrderRecord(file)
}
