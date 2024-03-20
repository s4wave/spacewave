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
	// create a random kvfile
	out := "demo.kvfile"
	targetSize := 20000 // Target size in bytes

	of, err := os.OpenFile(out, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer of.Close()

	var currentSize uint64
	keyIterator := func() (key []byte, err error) {
		if currentSize >= uint64(targetSize) {
			return nil, io.EOF
		}
		key = make([]byte, 8)
		binary.BigEndian.PutUint64(key, currentSize)
		return key, nil
	}

	valIterator := func(wr io.Writer, key []byte) (uint64, error) {
		value := make([]byte, 100) // Adjust the value size as needed
		for i := range value {
			value[i] = byte(currentSize % 256)
		}
		n, err := wr.Write(value)
		currentSize += uint64(n)
		return uint64(n), err
	}

	err = kvfile.WriteIterator(of, keyIterator, valIterator)
	if err != nil {
		return err
	}

	fmt.Printf("Written %d bytes\n", currentSize)
	return nil
}
