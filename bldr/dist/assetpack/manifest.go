package assetpack

import (
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// Part describes one physical file in a split asset pack.
type Part struct {
	URL  string
	Size int64
}

// MarshalParts formats the physical files backing one logical kvfile.
func MarshalParts(parts []Part) ([]byte, error) {
	var output strings.Builder
	for _, part := range parts {
		if part.URL == "" || strings.ContainsAny(part.URL, "\r\n") || part.Size <= 0 {
			return nil, errors.New("asset pack part has invalid URL or size")
		}
		output.WriteString(strconv.FormatInt(part.Size, 10))
		output.WriteByte(' ')
		output.WriteString(part.URL)
		output.WriteByte('\n')
	}
	return []byte(output.String()), nil
}

// UnmarshalParts parses the physical files backing one logical kvfile.
func UnmarshalParts(data []byte) ([]Part, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	parts := make([]Part, 0, len(lines))
	for _, line := range lines {
		sizeText, url, ok := strings.Cut(line, " ")
		if !ok || url == "" {
			return nil, errors.New("invalid asset pack part")
		}
		size, err := strconv.ParseInt(sizeText, 10, 64)
		if err != nil || size <= 0 {
			return nil, errors.New("asset pack part has invalid size")
		}
		parts = append(parts, Part{URL: url, Size: size})
	}
	if len(parts) == 0 {
		return nil, errors.New("asset pack has no parts")
	}
	return parts, nil
}
