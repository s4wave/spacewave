package git_block

import (
	"io"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/storer"
)

type sliceEncodedObjectIter struct {
	objects []plumbing.EncodedObject
	idx     int
}

func newSliceEncodedObjectIter(objects []plumbing.EncodedObject) *sliceEncodedObjectIter {
	return &sliceEncodedObjectIter{objects: objects}
}

func (i *sliceEncodedObjectIter) Next() (plumbing.EncodedObject, error) {
	if i.idx >= len(i.objects) {
		return nil, io.EOF
	}
	obj := i.objects[i.idx]
	i.idx++
	return obj, nil
}

func (i *sliceEncodedObjectIter) ForEach(cb func(plumbing.EncodedObject) error) error {
	for {
		obj, err := i.Next()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if err := cb(obj); err != nil {
			if err == storer.ErrStop {
				return nil
			}
			return err
		}
	}
}

func (i *sliceEncodedObjectIter) Close() {
}

// _ is a type assertion
var _ storer.EncodedObjectIter = ((*sliceEncodedObjectIter)(nil))
