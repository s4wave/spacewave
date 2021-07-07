package unixfs_block

import (
	"context"
	"io"
)

// ReaddirAll reads all directory entries to a map.
func ReaddirAll(ctx context.Context, f *FSTree) (map[string]*Dirent, error) {
	dstream, err := f.Readdir()
	if err != nil {
		return nil, err
	}
	m := make(map[string]*Dirent)
	if dstream == nil {
		return m, nil
	}
	for {
		ent := dstream.GetEntry()
		if ent == nil {
			break
		}
		m[ent.GetName()] = ent
		if !dstream.HasNext() {
			break
		}
		if err := dstream.Next(ctx); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}
	return m, nil
}
