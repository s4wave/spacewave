package space_http_export

import (
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	space_projectedpath "github.com/s4wave/spacewave/core/space/unixfs/projectedpath"
)

// ControllerID is the controller identifier.
const ControllerID = "space/http/export"

// exportPathPrefix is the URL path prefix for projected subtree export endpoints.
const exportPathPrefix = "/export/"

// exportBatchPathPrefix is the URL path prefix for batch export endpoints.
const exportBatchPathPrefix = "/export-batch/"

type exportRequest struct {
	sessionIdx     uint32
	sharedObjectID string
	projectedPath  string
}

type batchExportRequest struct {
	sessionIdx     uint32
	sharedObjectID string
	basePath       string
	filename       string
	paths          []string
}

func parseExportURL(path string) (*exportRequest, error) {
	projected, err := space_projectedpath.Parse(strings.TrimPrefix(path, exportPathPrefix))
	if err != nil {
		return nil, err
	}
	return &exportRequest{
		sessionIdx:     projected.SessionIdx,
		sharedObjectID: projected.SharedObjectID,
		projectedPath:  projected.Path,
	}, nil
}

func parseBatchExportURL(path string) (*batchExportRequest, error) {
	rest := strings.TrimPrefix(path, exportBatchPathPrefix)
	lastSlash := strings.LastIndex(rest, "/")
	if lastSlash <= 0 || lastSlash == len(rest)-1 {
		return nil, errors.New("invalid export-batch URL format")
	}
	filename, err := url.PathUnescape(rest[lastSlash+1:])
	if err != nil {
		return nil, errors.Wrap(err, "decode export batch filename")
	}

	secondSlash := strings.LastIndex(rest[:lastSlash], "/")
	if secondSlash <= 0 || secondSlash == len(rest[:lastSlash])-1 {
		return nil, errors.New("invalid export-batch URL format")
	}
	basePath := rest[:secondSlash]
	projected, err := space_projectedpath.Parse(basePath)
	if err != nil {
		return nil, err
	}

	paths, err := decodeBatchRequest(rest[secondSlash+1 : lastSlash])
	if err != nil {
		return nil, err
	}

	return &batchExportRequest{
		sessionIdx:     projected.SessionIdx,
		sharedObjectID: projected.SharedObjectID,
		basePath:       projected.Path,
		filename:       filename,
		paths:          paths,
	}, nil
}

func buildExportFilename(projectedPath string) string {
	base := projectedPath
	if idx := strings.LastIndex(projectedPath, "/"); idx >= 0 {
		base = projectedPath[idx+1:]
	}
	if base == "-" {
		base = path.Base(path.Dir(projectedPath))
	}
	if base == "" || base == "." {
		return "export.zip"
	}
	return base + ".zip"
}

func resolveProjectedExportTarget(req *exportRequest) (lookupPath string, zipRoot string) {
	spaceRoot := "u/" + strconv.FormatUint(uint64(req.sessionIdx), 10) + "/so/" + req.sharedObjectID
	if req.projectedPath == spaceRoot {
		return spaceRoot + "/-", ""
	}
	if req.projectedPath == spaceRoot+"/-" {
		return req.projectedPath, ""
	}

	zipRoot = path.Base(req.projectedPath)
	if zipRoot == "-" {
		zipRoot = path.Base(path.Dir(req.projectedPath))
	}
	return req.projectedPath, zipRoot
}
