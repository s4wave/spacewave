package space_exec

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"

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
	typeID, typeIDErr := world_types.GetObjectType(ctx, h.ws, objKey)
	if typeIDErr != nil {
		h.le.WithError(typeIDErr).Warn("zip export: object type lookup failed, exporting raw block")
	}
	fsType, hasType, err := unixfs_world.LookupFsType(ctx, h.ws, objKey)
	if err == nil && hasType {
		return h.buildFSZip(ctx, w, objKey, fsType)
	}
	if err != nil {
		h.le.WithError(err).Debug("zip export: fs type lookup failed, exporting raw block")
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

	return unixfs.WriteZipArchive(ctx, w, fsh, "")
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
