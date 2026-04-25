package space_unixfs

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// ProjectedPath is a parsed projected filesystem path.
type ProjectedPath struct {
	// SessionIdx is the projected session index.
	SessionIdx uint32
	// SharedObjectID is the projected shared object identifier.
	SharedObjectID string
	// Path is the normalized projected path.
	Path string
}

// ParseProjectedPath parses u/{idx}/so/{soId}... into projected path metadata.
func ParseProjectedPath(path string) (*ProjectedPath, error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil, errors.New("empty projected path")
	}

	rawSegs := strings.Split(path, "/")
	if len(rawSegs) < 4 || rawSegs[0] != "u" || rawSegs[2] != "so" {
		return nil, errors.New("invalid projected path format")
	}

	idx, err := strconv.ParseUint(rawSegs[1], 10, 32)
	if err != nil {
		return nil, errors.Wrap(err, "parse session index")
	}

	decodedSegs := make([]string, len(rawSegs))
	for i, seg := range rawSegs {
		decoded, err := url.PathUnescape(seg)
		if err != nil {
			return nil, errors.Wrap(err, "decode projected path segment")
		}
		decodedSegs[i] = decoded
	}

	return &ProjectedPath{
		SessionIdx:     uint32(idx),
		SharedObjectID: decodedSegs[3],
		Path:           strings.Join(decodedSegs, "/"),
	}, nil
}
