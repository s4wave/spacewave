package space_exec

import (
	"context"
	"io"
	"strconv"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
)

// UnixfsReadConfigID is the config ID for the unixfs-read handler.
const UnixfsReadConfigID = "space-exec/unixfs-read"

// unixfsReadConfig holds the parsed config for the unixfs-read handler.
type unixfsReadConfig struct {
	// objectKey is the world object key of the unixfs object.
	objectKey string
	// filePath is the path within the unixfs tree to read.
	filePath string
}

// parseUnixfsReadConfig parses the config from JSON bytes.
// Expected format: {"object_key": "...", "file_path": "..."}
func parseUnixfsReadConfig(data []byte) (*unixfsReadConfig, error) {
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
	filePath := string(v.GetStringBytes("file_path"))
	return &unixfsReadConfig{objectKey: objKey, filePath: filePath}, nil
}

// unixfsReadHandler reads a file from a unixfs world object and outputs a
// snapshot of the source object with file metadata in the execution log.
type unixfsReadHandler struct {
	le     *logrus.Entry
	ws     world.WorldState
	handle forge_target.ExecControllerHandle
	conf   *unixfsReadConfig
}

// Execute reads the file and sets the output.
func (h *unixfsReadHandler) Execute(ctx context.Context) error {
	objKey := h.conf.objectKey
	filePath := h.conf.filePath

	// Read the file contents from the unixfs object.
	data, err := readUnixfsFile(ctx, h.le, h.ws, objKey, filePath)
	if err != nil {
		return err
	}

	// Log file metadata.
	_ = h.handle.WriteLog(ctx, "info", "read "+strconv.Itoa(len(data))+" bytes from "+objKey+":"+filePath)

	// Build a snapshot of the source object as the output.
	obj, err := world.MustGetObject(ctx, h.ws, objKey)
	if err != nil {
		return errors.Wrap(err, "get source object for snapshot")
	}
	rootRef, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		return errors.Wrap(err, "get source object root ref")
	}

	snapshot := &forge_value.WorldObjectSnapshot{
		Key:     objKey,
		RootRef: rootRef,
		Rev:     rev,
	}
	outps := forge_value.ValueSlice{
		forge_value.NewValueWithWorldObjectSnapshot("source", snapshot),
	}
	return h.handle.SetOutputs(ctx, outps, true)
}

// readUnixfsFile reads the full contents of a file from a unixfs world object.
func readUnixfsFile(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	objKey string,
	filePath string,
) ([]byte, error) {
	fsType, _, err := unixfs_world.LookupFsType(ctx, ws, objKey)
	if err != nil {
		return nil, errors.Wrap(err, "lookup fs type")
	}

	fsCursor := unixfs_world.NewFSCursor(le, ws, objKey, fsType, nil, false)
	fsh, err := unixfs.NewFSHandle(fsCursor)
	if err != nil {
		fsCursor.Release()
		return nil, errors.Wrap(err, "create fs handle")
	}
	defer fsh.Release()

	// Navigate to the target file.
	target := fsh
	if filePath != "" {
		child, _, err := fsh.LookupPath(ctx, filePath)
		if err != nil {
			if child != nil {
				child.Release()
			}
			return nil, errors.Wrap(err, "lookup path")
		}
		target = child
		defer child.Release()
	}

	size, err := target.GetSize(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get file size")
	}

	buf := make([]byte, size)
	n, err := target.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		return nil, errors.Wrap(err, "read file")
	}
	return buf[:n], nil
}

// NewUnixfsReadHandler constructs a unixfs-read space handler.
func NewUnixfsReadHandler(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	handle forge_target.ExecControllerHandle,
	inputs forge_target.InputMap,
	configData []byte,
) (Handler, error) {
	conf, err := parseUnixfsReadConfig(configData)
	if err != nil {
		return nil, errors.Wrap(err, "parse unixfs-read config")
	}
	return &unixfsReadHandler{
		le:     le,
		ws:     ws,
		handle: handle,
		conf:   conf,
	}, nil
}

// RegisterUnixfsRead registers the unixfs-read handler in the registry.
func RegisterUnixfsRead(r *Registry) {
	r.Register(UnixfsReadConfigID, NewUnixfsReadHandler)
}

// _ is a type assertion
var _ Handler = (*unixfsReadHandler)(nil)
