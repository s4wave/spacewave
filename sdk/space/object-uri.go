package s4wave_space

import (
	"path"
	"strconv"
	"strings"
)

// SubpathDelimiter is the delimiter between segments in a spacewave URI.
const SubpathDelimiter = "/-/"

// PathSeparator is the path separator.
const PathSeparator = "/"

// SpacewaveURI is a parsed spacewave URI.
// Full form: /u/{session_idx}/so/{space_id}/-/{objectKey}/-/{path}/-/{nested}
// Short form: objectKey/-/path (defaults session=1, space="")
type SpacewaveURI struct {
	// SessionIdx is the session index.
	SessionIdx uint32
	// SpaceID is the space identifier.
	SpaceID string
	// Segments contains the path segments split by /-/ delimiters.
	// [0]=objectKey, [1]=path, [2+]=nested
	Segments []string
}

// ParseSpacewaveURI parses a spacewave URI string.
//
// When the argument starts with /u/, the full form is parsed:
//
//	/u/{session_idx}/so/{space_id}/-/{objectKey}/-/{path}/-/{nested}
//
// When the argument contains /-/ without a /u/ prefix, defaults are applied
// (session=1, space="") and the string is split on /-/ delimiters.
//
// A plain string without /-/ is treated as an object key only.
func ParseSpacewaveURI(uri string) (SpacewaveURI, error) {
	cleaned := path.Clean("/" + uri)
	uri = strings.TrimPrefix(cleaned, "/")

	if strings.HasPrefix(uri, "u/") {
		return parseFullURI(uri)
	}

	result := SpacewaveURI{SessionIdx: 1}
	result.Segments = splitSegments(uri)
	return result, nil
}

// parseFullURI parses the full /u/{idx}/so/{space_id}/... form.
func parseFullURI(uri string) (SpacewaveURI, error) {
	// uri starts with "u/"
	rest := strings.TrimPrefix(uri, "u/")

	// extract session index
	idx, rest := splitFirst(rest, "/")
	sessIdx, err := strconv.ParseUint(idx, 10, 32)
	if err != nil {
		return SpacewaveURI{}, err
	}
	result := SpacewaveURI{SessionIdx: uint32(sessIdx)}

	if rest == "" {
		return result, nil
	}

	// check for "so/" prefix for space ID
	if after, ok := strings.CutPrefix(rest, "so/"); ok {
		rest = after
		// space ID is everything up to the first /-/ or end of string
		delimIdx := strings.Index(rest, SubpathDelimiter)
		if delimIdx == -1 {
			result.SpaceID = rest
			return result, nil
		}
		result.SpaceID = rest[:delimIdx]
		rest = rest[delimIdx+len(SubpathDelimiter):]
	}

	if rest != "" {
		result.Segments = splitSegments(rest)
	}

	return result, nil
}

// splitSegments splits a URI remainder on /-/ delimiters into segments.
// Trailing "/-" markers are cleaned.
func splitSegments(s string) []string {
	// strip trailing "/-" marker
	s = strings.TrimSuffix(s, "/-")

	if s == "" {
		return nil
	}

	parts := strings.Split(s, SubpathDelimiter)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		// clean trailing "-" markers from individual segments
		if p == "-" {
			continue
		}
		p = strings.TrimSuffix(p, "/-")
		result = append(result, p)
	}
	return result
}

// splitFirst splits s on the first occurrence of sep.
// Returns (s, "") if sep is not found.
func splitFirst(s, sep string) (string, string) {
	before, after, ok := strings.Cut(s, sep)
	if !ok {
		return s, ""
	}
	return before, after
}

// ObjectURI is a parsed object URI containing an object key and subpath.
type ObjectURI struct {
	// ObjectKey is the object key portion of the URI.
	ObjectKey string
	// Path is the subpath portion of the URI.
	Path string
}

// ParseObjectURI parses a URI path to extract object key and subpath components.
// Uses /-/ as delimiter between object key and subpath.
// A trailing /- is treated the same as /-/ with empty path.
func ParseObjectURI(uri string) ObjectURI {
	cleaned := path.Clean("/" + uri)
	uri = strings.TrimPrefix(cleaned, "/")

	// if URI starts with subpath delimiter, remove it
	trimmed := strings.TrimPrefix(SubpathDelimiter, "/")
	if strings.HasPrefix(uri, trimmed) {
		uri = uri[len(trimmed):]
	}

	delimIdx := strings.Index(uri, SubpathDelimiter)
	if delimIdx != -1 {
		objectKey := uri[:delimIdx]
		p := uri[delimIdx+len(SubpathDelimiter):]
		if p == "-" {
			p = ""
		} else {
			p = strings.TrimSuffix(p, "/-")
		}
		return ObjectURI{ObjectKey: objectKey, Path: p}
	}

	uri = strings.TrimSuffix(uri, "/-")
	return ObjectURI{ObjectKey: uri}
}

// JoinObjectURIPath joins path parts into a single path string.
// Empty segments are filtered and a trailing "-" is removed.
func JoinObjectURIPath(parts []string, absolute bool) string {
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p != "" {
			filtered = append(filtered, p)
		}
	}

	if n := len(filtered); n > 0 && filtered[n-1] == "-" {
		filtered = filtered[:n-1]
	}

	joined := strings.Join(filtered, PathSeparator)
	if joined != "" && absolute {
		return PathSeparator + joined
	}
	return joined
}
