package gdrivetest

// Call records an HTTP action without credentials or request bodies.
type Call struct {
	// Method is the HTTP method.
	Method string
	// Path is the endpoint path without its query.
	Path string
	// Range is the media or upload byte range.
	Range string
}
