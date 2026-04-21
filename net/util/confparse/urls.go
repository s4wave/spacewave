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

// ValidateURL checks if a URL is set and valid.
func ValidateURL(uri string, allowEmpty bool) error {
	url, err := ParseURL(uri)
	if err == nil && (url == nil && !allowEmpty) {
		err = errors.New("url cannot be empty")
	}
	return err
}

// ParseURLs parses a list of urls.
//
// Removes any empty values.
func ParseURLs(urlStrs []string, allowEmpty bool) ([]*url.URL, error) {
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
