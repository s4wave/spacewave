package space_exec

import (
	"context"
	"encoding/hex"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// FileHashConfigID is the config ID for the file-hash handler.
const FileHashConfigID = "space-exec/file-hash"

// fileHashConfig holds the parsed config for the file-hash handler.
type fileHashConfig struct {
	// objectKey is the world object key of the unixfs object.
	objectKey string
	// filePath is the path within the unixfs tree to hash.
	filePath string
}

// parseFileHashConfig parses the config from JSON bytes.
// Expected format: {"object_key": "...", "file_path": "..."}
func parseFileHashConfig(data []byte) (*fileHashConfig, error) {
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
	return &fileHashConfig{objectKey: objKey, filePath: filePath}, nil
}

// fileHashHandler reads a file from a unixfs world object, computes its blake3
// hash, and writes the hex digest to the execution log.
type fileHashHandler struct {
	le     *logrus.Entry
	ws     world.WorldState
	handle forge_target.ExecControllerHandle
	conf   *fileHashConfig
}

// Execute reads the file, computes the hash, and logs the result.
func (h *fileHashHandler) Execute(ctx context.Context) error {
	data, err := readUnixfsFile(ctx, h.le, h.ws, h.conf.objectKey, h.conf.filePath)
	if err != nil {
		return err
	}

	hasher := blake3.New()
	_, _ = hasher.Write(data)
	digest := hex.EncodeToString(hasher.Sum(nil))

	return h.handle.WriteLog(ctx, "info", "blake3:"+digest)
}

// NewFileHashHandler constructs a file-hash space handler.
func NewFileHashHandler(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	handle forge_target.ExecControllerHandle,
	inputs forge_target.InputMap,
	configData []byte,
) (Handler, error) {
	conf, err := parseFileHashConfig(configData)
	if err != nil {
		return nil, errors.Wrap(err, "parse file-hash config")
	}
	return &fileHashHandler{
		le:     le,
		ws:     ws,
		handle: handle,
		conf:   conf,
	}, nil
}

// RegisterFileHash registers the file-hash handler in the registry.
func RegisterFileHash(r *Registry) {
	r.Register(FileHashConfigID, NewFileHashHandler)
}

// _ is a type assertion
var _ Handler = (*fileHashHandler)(nil)
