package block_store_gdrive

import (
	"strconv"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// folderType identifies a Drive directory.
const folderType = "application/vnd.google-apps.folder"

// File is the verified metadata of one immutable Drive object.
type File struct {
	// ID is Drive's opaque file identifier.
	ID string
	// Name is the object's final path segment.
	Name string
	// Size is the complete object's byte count.
	Size int64
	// MD5 is Drive's checksum of its stored bytes.
	MD5 string
	// Key is the complete object key recorded on creation.
	Key string
	// SHA256 is the content digest recorded on creation.
	SHA256 string
	// ExpectedMD5 is the client checksum recorded on creation.
	ExpectedMD5 string
	// ExpectedSize is the client byte count recorded on creation.
	ExpectedSize string
	// Folder identifies a directory rather than an uploaded object.
	Folder bool
}

// ParseFile reads Drive metadata without reflection or retaining parser memory.
func ParseFile(v *fastjson.Value) (File, error) {
	// Copy parser-owned strings before the page parser is released.
	f := File{
		ID: string(v.GetStringBytes("id")), Name: string(v.GetStringBytes("name")),
		MD5: string(v.GetStringBytes("md5Checksum")), Key: string(v.GetStringBytes("appProperties", "key")),
		SHA256:       string(v.GetStringBytes("appProperties", "sha256")),
		ExpectedMD5:  string(v.GetStringBytes("appProperties", "md5")),
		ExpectedSize: string(v.GetStringBytes("appProperties", "size")),
		Folder:       string(v.GetStringBytes("mimeType")) == folderType,
	}
	if raw := v.GetStringBytes("size"); len(raw) != 0 {
		var err error
		f.Size, err = strconv.ParseInt(string(raw), 10, 64)
		if err != nil {
			return File{}, errors.Wrap(err, "Drive object size")
		}
	}
	return f, nil
}

// Verified reports whether Drive's final metadata matches the create request.
func (f File) Verified() bool {
	return f.ID != "" && f.MD5 != "" && f.MD5 == f.ExpectedMD5 && strconv.FormatInt(f.Size, 10) == f.ExpectedSize && f.SHA256 != "" && f.Key != ""
}

// metadata encodes the fields Drive accepts when creating a file.
func metadata(name, parent, key, md5sum, sha256sum string, size int, folder bool) []byte {
	// Build explicit JSON fields for Drive's external contract.
	var a fastjson.Arena
	v := a.NewObject()
	v.Set("name", a.NewString(name))
	parents := a.NewArray()
	parents.SetArrayItem(0, a.NewString(parent))
	v.Set("parents", parents)
	if folder {
		v.Set("mimeType", a.NewString(folderType))
	} else {
		props := a.NewObject()
		props.Set("key", a.NewString(key))
		props.Set("md5", a.NewString(md5sum))
		props.Set("sha256", a.NewString(sha256sum))
		props.Set("size", a.NewString(strconv.Itoa(size)))
		v.Set("appProperties", props)
	}
	return v.MarshalTo(nil)
}
