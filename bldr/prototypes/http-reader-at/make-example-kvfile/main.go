package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/aperturerobotics/go-kvfile"
)

func main() {
	if err := run(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run() error {
	// Configure the output path and size limit for the sample KVFile.
	// create a random kvfile
	out := "demo.kvfile"
	targetSize := 5 * 1024 * 512 // Target size in bytes

	// Open the sample KVFile for a complete rewrite and retain its handle.
	of, err := os.OpenFile(out, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer of.Close()

	// Track bytes written so the iterators share one KVFile offset.
	var currentSize uint64
	keyIterator := func() (key []byte, err error) {
		// Stop producing keys once the requested KVFile size is reached.
		if currentSize >= uint64(targetSize) { //nolint:gosec
			return nil, io.EOF
		}

		// Encode the current byte offset as the next fixed-width key.
		key = make([]byte, 8)
		binary.BigEndian.PutUint64(key, currentSize)
		return key, nil
	}

	// Generate fixed-size values and advance the sample KVFile offset.
	valIterator := func(wr io.Writer, key []byte) (uint64, error) {
		// Fill one value with the byte pattern for the current offset.
		value := make([]byte, 100) // Adjust the value size as needed
		for i := range value {
			value[i] = byte(currentSize % 256)
		}

		// Write the generated value and advance the shared byte count.
		n, err := wr.Write(value)
		n64 := uint64(n) //nolint:gosec
		currentSize += n64
		return n64, err
	}

	// Stream generated keys and values into the sample KVFile.
	err = kvfile.WriteIterator(of, keyIterator, valIterator)
	if err != nil {
		return err
	}

	// Report the number of bytes written to the KVFile.
	fmt.Printf("Written %d bytes\n", currentSize)
	return nil
}
