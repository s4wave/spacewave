package space_http_export

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/base64"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

func decodeBatchRequest(payload string) ([]string, error) {
	encoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, errors.Wrap(err, "decode batch payload")
	}

	if data, err := inflateData(encoded); err == nil {
		paths, err := decodeBatchRequestData(data)
		if err == nil {
			return paths, nil
		}
	}
	return decodeBatchRequestData(encoded)
}

func decodeBatchRequestData(data []byte) ([]string, error) {
	var req ExportBatchRequest
	if err := req.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode batch request")
	}
	return normalizeBatchPaths(req.GetPaths())
}

func inflateData(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	if out, err := readCompressed(zlib.NewReader(bytes.NewReader(data))); err == nil {
		return out, nil
	}
	return readCompressed(flate.NewReader(bytes.NewReader(data)), nil)
}

func readCompressed(reader io.ReadCloser, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func normalizeBatchPaths(relPaths []string) ([]string, error) {
	if len(relPaths) == 0 {
		return nil, errors.New("batch request requires at least one path")
	}

	seen := make(map[string]struct{}, len(relPaths))
	normalized := make([]string, 0, len(relPaths))
	for _, relPath := range relPaths {
		cleanPath, err := normalizeBatchPath(relPath)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[cleanPath]; ok {
			continue
		}
		seen[cleanPath] = struct{}{}
		normalized = append(normalized, cleanPath)
	}
	slices.Sort(normalized)
	return normalized, nil
}

func normalizeBatchPath(relPath string) (string, error) {
	trimmedPath := strings.TrimSpace(relPath)
	if trimmedPath == "" {
		return "", errors.New("batch path is empty")
	}
	if strings.HasPrefix(trimmedPath, "/") {
		return "", errors.New("batch path must be relative")
	}

	cleanPath := path.Clean(trimmedPath)
	if cleanPath == "." {
		return "", errors.New("batch path must identify a descendant")
	}
	if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
		return "", errors.New("batch path escapes base path")
	}
	return cleanPath, nil
}
