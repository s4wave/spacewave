package unixfs_v86fs

import (
	"io"
	"io/fs"
	"testing"

	"github.com/pkg/errors"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

func TestErrnoFromError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want uint32
	}{
		{
			name: "nil",
			err:  nil,
			want: 0,
		},
		{
			name: "unixfs not exist",
			err:  unixfs_errors.ErrNotExist,
			want: enoent,
		},
		{
			name: "fs not exist",
			err:  fs.ErrNotExist,
			want: enoent,
		},
		{
			name: "unixfs exist",
			err:  unixfs_errors.ErrExist,
			want: eexist,
		},
		{
			name: "fs exist",
			err:  fs.ErrExist,
			want: eexist,
		},
		{
			name: "unixfs not directory",
			err:  unixfs_errors.ErrNotDirectory,
			want: enotdir,
		},
		{
			name: "unixfs read only",
			err:  unixfs_errors.ErrReadOnly,
			want: erofs,
		},
		{
			name: "fs invalid",
			err:  fs.ErrInvalid,
			want: einval,
		},
		{
			name: "eof",
			err:  io.EOF,
			want: 0,
		},
		{
			name: "wrapped read only",
			err:  errors.Wrap(unixfs_errors.ErrReadOnly, "x"),
			want: erofs,
		},
		{
			name: "generic",
			err:  errors.New("boom"),
			want: enosys,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := errnoFromError(test.err)
			if got != test.want {
				t.Fatalf("errnoFromError() = %d, want %d", got, test.want)
			}
		})
	}
}
