//go:build !goscript

package plugin_host_wazero_quickjs

import (
	"crypto/rand"
	"io"
	"io/fs"

	wazero_exp_sys "github.com/tetratelabs/wazero/experimental/sys"
	wazero_sys "github.com/tetratelabs/wazero/sys"
)

// devFS is the /dev filesystem of a QuickJS plugin. QuickJS has no host
// bindings beyond WASI files, so devices carry what the plugin needs from the
// host:
//
//	out:     write-only, the plugin's async output stream.
//	urandom: read-only, cryptographically secure random bytes.
type devFS struct {
	wazero_exp_sys.UnimplementedFS
	out io.Writer
}

// newDevFS creates a devFS that writes /dev/out to out.
func newDevFS(out io.Writer) *devFS {
	return &devFS{out: out}
}

// devEntries lists the /dev directory.
var devEntries = []wazero_exp_sys.Dirent{
	{Name: "out", Type: fs.ModeCharDevice},
	{Name: "urandom", Type: fs.ModeCharDevice},
}

// OpenFile opens the /dev directory or one of its devices.
func (d *devFS) OpenFile(name string, flag wazero_exp_sys.Oflag, _ fs.FileMode) (wazero_exp_sys.File, wazero_exp_sys.Errno) {
	writable := flag&(wazero_exp_sys.O_WRONLY|wazero_exp_sys.O_RDWR) != 0
	switch name {
	case ".":
		if writable {
			return nil, wazero_exp_sys.EACCES
		}
		return &devDirFile{}, 0
	case "out":
		return &devOutFile{w: d.out}, 0
	case "urandom":
		if writable {
			return nil, wazero_exp_sys.EACCES
		}
		return &devRandomFile{}, 0
	default:
		return nil, wazero_exp_sys.ENOENT
	}
}

// Lstat returns the status of the /dev directory or one of its devices.
func (d *devFS) Lstat(name string) (wazero_sys.Stat_t, wazero_exp_sys.Errno) {
	switch name {
	case ".":
		return devDirStat, 0
	case "out":
		return devOutStat, 0
	case "urandom":
		return devRandomStat, 0
	default:
		return wazero_sys.Stat_t{}, wazero_exp_sys.ENOENT
	}
}

// Stat returns the same status as Lstat: /dev has no links.
func (d *devFS) Stat(name string) (wazero_sys.Stat_t, wazero_exp_sys.Errno) {
	return d.Lstat(name)
}

var (
	devDirStat    = wazero_sys.Stat_t{Mode: fs.ModeDir | 0o555}
	devOutStat    = wazero_sys.Stat_t{Mode: fs.ModeCharDevice | 0o200}
	devRandomStat = wazero_sys.Stat_t{Mode: fs.ModeCharDevice | 0o444}
)

// devDirFile is the open /dev directory.
type devDirFile struct {
	wazero_exp_sys.UnimplementedFile
	// read is the number of entries Readdir already returned.
	read int
}

// IsDir reports that /dev is a directory.
func (f *devDirFile) IsDir() (bool, wazero_exp_sys.Errno) {
	return true, 0
}

// Stat returns the status of the /dev directory.
func (f *devDirFile) Stat() (wazero_sys.Stat_t, wazero_exp_sys.Errno) {
	return devDirStat, 0
}

// Readdir returns up to n of the remaining entries, or all of them when n <= 0.
func (f *devDirFile) Readdir(n int) ([]wazero_exp_sys.Dirent, wazero_exp_sys.Errno) {
	rest := devEntries[f.read:]
	if n > 0 && n < len(rest) {
		rest = rest[:n]
	}
	f.read += len(rest)
	return rest, 0
}

// devOutFile is the open /dev/out device.
type devOutFile struct {
	wazero_exp_sys.UnimplementedFile
	w io.Writer
}

// Stat returns the status of /dev/out.
func (f *devOutFile) Stat() (wazero_sys.Stat_t, wazero_exp_sys.Errno) {
	return devOutStat, 0
}

// Write writes buf to the plugin's output stream.
func (f *devOutFile) Write(buf []byte) (int, wazero_exp_sys.Errno) {
	n, err := f.w.Write(buf)
	if err != nil {
		return n, wazero_exp_sys.EIO
	}
	return n, 0
}

// devRandomFile is the open /dev/urandom device.
type devRandomFile struct {
	wazero_exp_sys.UnimplementedFile
}

// Stat returns the status of /dev/urandom.
func (f *devRandomFile) Stat() (wazero_sys.Stat_t, wazero_exp_sys.Errno) {
	return devRandomStat, 0
}

// Read fills buf with random bytes from crypto/rand, which never fails.
func (f *devRandomFile) Read(buf []byte) (int, wazero_exp_sys.Errno) {
	rand.Read(buf)
	return len(buf), 0
}

// _ is a type assertion
var (
	_ wazero_exp_sys.FS   = (*devFS)(nil)
	_ wazero_exp_sys.File = (*devDirFile)(nil)
	_ wazero_exp_sys.File = (*devOutFile)(nil)
	_ wazero_exp_sys.File = (*devRandomFile)(nil)
)
