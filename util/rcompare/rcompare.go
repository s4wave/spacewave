package rcompare

import (
	"bytes"
	"io"
)

// CompareReadersEqual compares two readers for equality.
func CompareReadersEqual(r1, r2 io.Reader) (bool, error) {
	buf1 := make([]byte, 1500)
	buf2 := make([]byte, 1500)
	for {
		n1, err := r1.Read(buf1[:cap(buf1)])
		if err != nil && err != io.EOF {
			return false, err
		}
		buf1 = buf1[:n1]

		n2, err := r2.Read(buf2[:cap(buf2)])
		if err != nil && err != io.EOF {
			return false, err
		}
		buf2 = buf2[:n2]
		if n1 != n2 {
			return false, nil
		}
		if n1 == 0 {
			return true, nil
		}
		if bytes.Compare(buf1, buf2) != 0 {
			return false, nil
		}
	}
}
