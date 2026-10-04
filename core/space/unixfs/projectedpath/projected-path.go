package projectedpath

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

// Parse parses u/{idx}/so/{soId}... into projected path metadata.
func Parse(path string) (*ProjectedPath, error) {
	// Normalize the projected path and reject an empty value.
	path = strings.Trim(path, "/")
	if path == "" {
		return nil, errors.New("empty projected path")
	}

	// Validate the session and SharedObject path prefix.
	rawSegs := strings.Split(path, "/")
	if len(rawSegs) < 4 || rawSegs[0] != "u" || rawSegs[2] != "so" {
		return nil, errors.New("invalid projected path format")
	}

	// Parse the projected session index as a 32-bit unsigned value.
	idx, err := strconv.ParseUint(rawSegs[1], 10, 32)
	if err != nil {
		return nil, errors.Wrap(err, "parse session index")
	}

	// Decode each path segment before constructing the projected path record.
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
