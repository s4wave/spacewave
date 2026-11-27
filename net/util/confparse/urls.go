package confparse

import (
	"net/url"

	"github.com/pkg/errors"
)

// ParseURL parses the url from a string.
// If there is no URL specified, returns nil, nil.
func ParseURL(uri string) (*url.URL, error) {
	if uri == "" {
		return nil, nil
	}
	return url.Parse(uri)
}

// ParseURLs parses a list of urls.
//
// Removes any empty values.
func ParseURLs(urlStrs []string, allowEmpty bool) ([]*url.URL, error) {
	// Parse each configured URL and omit allowed empty entries.
	urls := make([]*url.URL, 0, len(urlStrs))
	for i, urlStr := range urlStrs {
		v, err := ParseURL(urlStr)
		if v == nil && err == nil && !allowEmpty {
			err = errors.Wrapf(errors.New("empty url"), "urls[%d]", i)
		}
		if err != nil {
			return nil, errors.Wrapf(err, "urls[%d]", i)
		}
		if v != nil {
			urls = append(urls, v)
		}
	}
	return urls, nil
}
