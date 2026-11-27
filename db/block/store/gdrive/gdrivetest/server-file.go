package gdrivetest

import (
	"crypto/md5" //nolint:gosec // The fixture computes the checksum Drive supplies.
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"

	"github.com/aperturerobotics/fastjson"
)

// fileMetadata returns Drive's final metadata, including independently computed MD5.
func (s *Server) fileMetadata(f *file) []byte {
	// Parse a fresh object so metadata never aliases mutable media state.
	v, _ := fastjson.ParseBytes(f.meta)
	var a fastjson.Arena
	v.Set("id", a.NewString(f.id))
	if string(v.GetStringBytes("mimeType")) != "application/vnd.google-apps.folder" {
		v.Set("size", a.NewString(strconv.Itoa(f.storedSize)))
		v.Set("md5Checksum", a.NewString(f.checksum))
	}
	return v.MarshalTo(nil)
}

// finishFile verifies bytes and, for a disk fixture, syncs them before publication.
func (s *Server) finishFile(f *file) error {
	// Compute integrity independently of the client's declared appProperties.
	digest := md5.Sum(f.data) //nolint:gosec // Match Drive's transfer checksum.
	f.storedSize, f.checksum = len(f.data), hex.EncodeToString(digest[:])
	if s.directory == "" {
		f.finished = true
		return nil
	}

	// Commit a disk fixture's complete bytes before publishing its metadata.
	f.path = filepath.Join(s.directory, f.id)
	out, err := os.OpenFile(f.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := out.Write(f.data); err != nil {
		_ = out.Close() // A failed unpublished fake upload remains an orphan.
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close() // A failed unpublished fake upload remains an orphan.
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	// Release the upload buffer only after the complete file reached durable storage.
	f.data, f.finished = nil, true
	return nil
}
