package binpkg

import (
	"archive/tar"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/plugin/wasivm/layer"
)

// Archive is the content of a GPKG binary package.
type Archive struct {
	// Description is the package's one-line description.
	Description string
	// Runtime holds the files the package needs to run.
	Runtime []*layer.Entry
	// Dev holds the headers, static libraries, build metadata and
	// documentation needed only to build against the package.
	Dev []*layer.Entry
}

// mergedUsr maps the top-level directories a merged-usr root links into /usr.
var mergedUsr = map[string]string{"bin": "usr/bin", "sbin": "usr/sbin", "lib": "usr/lib"}

// devPrefixes are path prefixes of headers, static libraries, build metadata
// and documentation.
var devPrefixes = []string{
	"include/",
	"usr/include/",
	"usr/lib/pkgconfig/",
	"usr/lib/wasm32-wasi/",
	"usr/share/aclocal/",
	"usr/share/doc/",
	"usr/share/info/",
	"usr/share/man/",
	"usr/share/pkgconfig/",
}

// ReadArchive reads the description and installed files of the GPKG binary
// package at gpkgPath. A package with no image, such as a virtual package,
// installs no files. Paths under /bin, /sbin and /lib move under /usr, and
// hard links become copies of their target, which content addressing stores
// once.
func ReadArchive(gpkgPath string) (*Archive, error) {
	// Open the outer package tar.
	f, err := os.Open(gpkgPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read the metadata and image archives inside the outer package tar.
	var arc Archive
	var image []*layer.Entry
	outer := tar.NewReader(f)
	for {
		hdr, err := outer.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, gpkgPath)
		}
		switch path.Base(hdr.Name) {
		case "metadata.tar.zst":
			arc.Description, err = readDescription(outer)
		case "image.tar.zst":
			image, err = readImage(outer)
		}
		if err != nil {
			return nil, errors.Wrapf(err, "%s: %s", gpkgPath, hdr.Name)
		}
	}
	arc.Runtime, arc.Dev = split(image)
	return &arc, nil
}

// readDescription returns the DESCRIPTION of a compressed metadata archive.
func readDescription(r io.Reader) (string, error) {
	// Decompress the archive and find the DESCRIPTION file.
	zr, err := zstd.NewReader(r)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if hdr.Name == "metadata/DESCRIPTION" {
			data, err := io.ReadAll(tr)
			return strings.TrimSpace(string(data)), err
		}
	}
}

// readImage returns the normalized entries of a compressed image archive,
// whose paths are under "image/".
func readImage(r io.Reader) ([]*layer.Entry, error) {
	// Decompress the archive.
	zr, err := zstd.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	// Convert each entry, remembering regular files for the hard links that
	// follow them.
	var out []*layer.Entry
	files := make(map[string]*layer.Entry)
	tr := tar.NewReader(zr)
	for {
		// Read the next tar header and normalize its path.
		hdr, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		p := normalizePath(strings.TrimPrefix(hdr.Name, "image/"))
		if p == "" {
			continue
		}
		ent := &layer.Entry{Path: p, Perm: fs.FileMode(hdr.Mode).Perm()} //nolint:gosec

		// Convert the entry by its tar type.
		switch hdr.Typeflag {
		case tar.TypeDir:
			ent.Type = fs.ModeDir
		case tar.TypeSymlink:
			ent.Type = fs.ModeSymlink
			ent.Target = hdr.Linkname
		case tar.TypeLink:
			src := files[normalizePath(strings.TrimPrefix(hdr.Linkname, "image/"))]
			if src == nil {
				return nil, errors.Errorf("hard link %s to missing %s", hdr.Name, hdr.Linkname)
			}
			ent.Data = src.Data
		case tar.TypeReg:
			ent.Data, err = io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			files[p] = ent
		default:
			continue
		}
		out = append(out, ent)
	}
}

// normalizePath cleans an image path and moves merged-usr directories.
func normalizePath(p string) string {
	p = strings.Trim(path.Clean("/"+p), "/")
	first, rest, _ := strings.Cut(p, "/")
	if usr, ok := mergedUsr[first]; ok {
		return strings.TrimSuffix(usr+"/"+rest, "/")
	}
	return p
}

// isDev reports whether a normalized path is build-time content.
func isDev(p string) bool {
	for _, prefix := range devPrefixes {
		if strings.HasPrefix(p+"/", prefix) {
			return true
		}
	}
	ext := path.Ext(p)
	return ext == ".a" || ext == ".la" || ext == ".h" ||
		(strings.HasPrefix(p, "usr/bin/") && strings.HasSuffix(p, "-config"))
}

// split divides image entries into runtime and dev parts. A directory goes to
// each part that holds something under it. Portage marks a directory that must
// exist empty with a .keep file, so no needed directory is lost.
func split(entries []*layer.Entry) (runtime, dev []*layer.Entry) {
	// Record which directories hold runtime and dev content.
	var used [2]map[string]bool
	used[0], used[1] = make(map[string]bool), make(map[string]bool)
	part := func(e *layer.Entry) int {
		if isDev(e.Path) {
			return 1
		}
		return 0
	}
	for _, e := range entries {
		if e.Type == fs.ModeDir {
			continue
		}
		for d := path.Dir(e.Path); d != "."; d = path.Dir(d) {
			used[part(e)][d] = true
		}
	}

	// Place each entry in its parts.
	var parts [2][]*layer.Entry
	for _, e := range entries {
		if e.Type != fs.ModeDir {
			parts[part(e)] = append(parts[part(e)], e)
			continue
		}
		for i := range parts {
			if used[i][e.Path] {
				parts[i] = append(parts[i], e)
			}
		}
	}
	return parts[0], parts[1]
}
