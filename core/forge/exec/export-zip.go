package space_exec

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
)

// ExportZipConfigID is the config ID for the export-zip handler.
const ExportZipConfigID = "space-exec/export-zip"

// exportZipReadChunkSize is the size of each read chunk when streaming file content.
const exportZipReadChunkSize = 256 * 1024

// exportZipConfig holds the parsed config for the export-zip handler.
type exportZipConfig struct {
	// objectKey is the world object key to export.
	objectKey string
}

// parseExportZipConfig parses the config from JSON bytes.
// Expected format: {"object_key": "..."}
func parseExportZipConfig(data []byte) (*exportZipConfig, error) {
	if len(data) == 0 {
		return nil, errors.New("empty config")
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		return nil, errors.Wrap(err, "parse config json")
	}
	objKey := string(v.GetStringBytes("object_key"))
	if objKey == "" {
		return nil, errors.New("object_key is required")
	}
	return &exportZipConfig{objectKey: objKey}, nil
}

// exportZipHandler exports a world object as a zip file and writes the zip
// blob as a forge output value.
type exportZipHandler struct {
	le     *logrus.Entry
	ws     world.WorldState
	handle forge_target.ExecControllerHandle
	conf   *exportZipConfig
}

// Execute reads the source object, zips its contents, writes the zip as a blob
// block, and outputs the blob reference.
func (h *exportZipHandler) Execute(ctx context.Context) error {
	objKey := h.conf.objectKey

	// Verify the object exists.
	_, found, err := h.ws.GetObject(ctx, objKey)
	if err != nil {
		return errors.Wrap(err, "get object")
	}
	if !found {
		return errors.Errorf("object not found: %s", objKey)
	}

	// Build zip data.
	var buf bytes.Buffer
	if err := h.buildZip(ctx, &buf, objKey); err != nil {
		return errors.Wrap(err, "build zip")
	}

	_ = h.handle.WriteLog(ctx, "info", "zip: "+strconv.Itoa(buf.Len())+" bytes from "+objKey)

	// Write zip bytes as a blob block in world storage.
	zipData := buf.Bytes()
	blobRef, err := world.AccessObject(ctx, h.ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs.SetRefAtCursor(nil, true)
		_, berr := blob.BuildBlobWithBytes(ctx, zipData, bcs)
		return berr
	})
	if err != nil {
		return errors.Wrap(err, "write zip blob")
	}

	outps := forge_value.ValueSlice{
		forge_value.NewValueWithBucketRef("zip", blobRef),
	}
	return h.handle.SetOutputs(ctx, outps, true)
}

// buildZip creates a zip archive of the object's contents.
// For FS-backed objects (unixfs), walks the file tree.
// For other objects, exports the raw block data as a single entry.
func (h *exportZipHandler) buildZip(ctx context.Context, w io.Writer, objKey string) error {
	// Check if the object is a unixfs FS type.
	typeID, _ := world_types.GetObjectType(ctx, h.ws, objKey)
	fsType, hasType, err := unixfs_world.LookupFsType(ctx, h.ws, objKey)
	if err == nil && hasType {
		return h.buildFSZip(ctx, w, objKey, fsType)
	}

	// Fall back to raw block export.
	return h.buildRawZip(ctx, w, objKey, typeID)
}

// buildFSZip creates a zip of a unixfs object's file tree.
func (h *exportZipHandler) buildFSZip(ctx context.Context, w io.Writer, objKey string, fsType unixfs_world.FSType) error {
	fsCursor := unixfs_world.NewFSCursor(h.le, h.ws, objKey, fsType, nil, false)
	fsh, err := unixfs.NewFSHandle(fsCursor)
	if err != nil {
		fsCursor.Release()
		return errors.Wrap(err, "create fs handle")
	}
	defer fsh.Release()

	zw := zip.NewWriter(w)
	if err := exportZipWalkAndZip(ctx, zw, fsh, ""); err != nil {
		zw.Close()
		return err
	}
	return zw.Close()
}

// buildRawZip creates a zip with the object's raw block data as a single entry.
func (h *exportZipHandler) buildRawZip(ctx context.Context, w io.Writer, objKey string, typeID string) error {
	objState, found, err := h.ws.GetObject(ctx, objKey)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	var bodyData []byte
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		data, _, ferr := bcs.Fetch(ctx)
		if ferr != nil {
			return ferr
		}
		bodyData = data
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "read object block")
	}

	ext := ".pb"
	if typeID != "" {
		ext = "." + strings.ReplaceAll(typeID, "/", "-") + ".pb"
	}

	zw := zip.NewWriter(w)
	header := &zip.FileHeader{
		Name:   sanitizeExportKey(objKey) + ext,
		Method: zip.Deflate,
	}
	fw, err := zw.CreateHeader(header)
	if err != nil {
		zw.Close()
		return err
	}
	if bodyData != nil {
		if _, err := fw.Write(bodyData); err != nil {
			zw.Close()
			return err
		}
	}
	return zw.Close()
}

// exportZipWalkAndZip recursively walks the FSHandle tree and writes zip entries.
func exportZipWalkAndZip(ctx context.Context, zw *zip.Writer, h *unixfs.FSHandle, prefix string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	type entry struct {
		name      string
		isDir     bool
		isSymlink bool
	}

	var entries []entry
	err := h.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		entries = append(entries, entry{
			name:      ent.GetName(),
			isDir:     ent.GetIsDirectory(),
			isSymlink: ent.GetIsSymlink(),
		})
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "readdir")
	}

	for _, e := range entries {
		entryPath := path.Join(prefix, e.name)
		if e.isDir {
			if err := exportZipDir(ctx, zw, h, e.name, entryPath); err != nil {
				return err
			}
			continue
		}
		if e.isSymlink {
			if err := exportZipSymlink(ctx, zw, h, e.name, entryPath); err != nil {
				return err
			}
			continue
		}
		if err := exportZipFile(ctx, zw, h, e.name, entryPath); err != nil {
			return err
		}
	}
	return nil
}

// exportZipDir writes a directory entry and recurses into it.
func exportZipDir(ctx context.Context, zw *zip.Writer, parent *unixfs.FSHandle, name string, entryPath string) error {
	child, err := parent.Lookup(ctx, name)
	if err != nil {
		return errors.Wrap(err, "lookup "+name)
	}
	defer child.Release()

	header := &zip.FileHeader{
		Name:     entryPath + "/",
		Method:   zip.Store,
		Modified: time.Time{},
	}
	header.SetMode(fs.ModeDir | 0o755)
	if _, err := zw.CreateHeader(header); err != nil {
		return errors.Wrap(err, "create dir header "+entryPath)
	}
	return exportZipWalkAndZip(ctx, zw, child, entryPath)
}

// exportZipSymlink writes a symlink entry with the target as content.
func exportZipSymlink(ctx context.Context, zw *zip.Writer, parent *unixfs.FSHandle, name string, entryPath string) error {
	target, isAbsolute, err := parent.Readlink(ctx, name)
	if err != nil {
		return errors.Wrap(err, "readlink "+name)
	}

	targetStr := strings.Join(target, "/")
	if isAbsolute {
		targetStr = "/" + targetStr
	}

	header := &zip.FileHeader{
		Name:     entryPath,
		Method:   zip.Store,
		Modified: time.Time{},
	}
	header.SetMode(fs.ModeSymlink | 0o777)
	w, err := zw.CreateHeader(header)
	if err != nil {
		return errors.Wrap(err, "create symlink header "+entryPath)
	}
	_, err = io.WriteString(w, targetStr)
	return err
}

// exportZipFile writes a regular file entry with deflate compression.
func exportZipFile(ctx context.Context, zw *zip.Writer, parent *unixfs.FSHandle, name string, entryPath string) error {
	child, err := parent.Lookup(ctx, name)
	if err != nil {
		return errors.Wrap(err, "lookup "+name)
	}
	defer child.Release()

	info, err := child.GetFileInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "getfileinfo "+entryPath)
	}

	header := &zip.FileHeader{
		Name:               entryPath,
		Method:             zip.Deflate,
		Modified:           info.ModTime(),
		UncompressedSize64: uint64(info.Size()),
	}
	header.SetMode(info.Mode())

	w, err := zw.CreateHeader(header)
	if err != nil {
		return errors.Wrap(err, "create file header "+entryPath)
	}

	buf := make([]byte, exportZipReadChunkSize)
	var offset int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := child.ReadAt(ctx, offset, buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return errors.Wrap(writeErr, "write "+entryPath)
			}
			offset += n
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return errors.Wrap(readErr, "read "+entryPath)
		}
		if n == 0 {
			break
		}
	}
	return nil
}

// sanitizeExportKey replaces unsafe characters in an object key for filenames.
func sanitizeExportKey(key string) string {
	return strings.NewReplacer("/", "_", "\"", "", "\\", "").Replace(key)
}

// NewExportZipHandler constructs an export-zip space handler.
func NewExportZipHandler(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	handle forge_target.ExecControllerHandle,
	inputs forge_target.InputMap,
	configData []byte,
) (Handler, error) {
	conf, err := parseExportZipConfig(configData)
	if err != nil {
		return nil, errors.Wrap(err, "parse export-zip config")
	}
	return &exportZipHandler{
		le:     le,
		ws:     ws,
		handle: handle,
		conf:   conf,
	}, nil
}

// RegisterExportZip registers the export-zip handler in the registry.
func RegisterExportZip(r *Registry) {
	r.Register(ExportZipConfigID, NewExportZipHandler)
}

// _ is a type assertion
var _ Handler = (*exportZipHandler)(nil)
