package main

import (
	_ "embed"
	"os"
	"strings"

	"github.com/google/go-licenses/v2/licenses"
)

// mitZero is the invariant license body from https://spdx.org/licenses/MIT-0.html.
//
//go:embed mit-zero.txt
var mitZero string

// classifier adds exact MIT-0 matching to the upstream license corpus.
type classifier struct {
	licenses.Classifier
}

// Identify accepts the MIT-0 body after optional title and copyright lines.
// Other text, including modified terms, remains subject to upstream matching.
func (c classifier) Identify(path string) ([]licenses.License, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Ignore only known header lines; compare every word of the license body.
	text := strings.TrimSpace(string(data))
	for text != "" {
		line, rest, _ := strings.Cut(text, "\n")
		header := strings.TrimSpace(line)
		if header != "" && header != "MIT No Attribution" && header != "MIT No Attribution License" && !strings.HasPrefix(header, "Copyright ") {
			break
		}
		text = rest
	}
	if strings.Join(strings.Fields(text), " ") == strings.Join(strings.Fields(mitZero), " ") {
		return []licenses.License{{Name: "MIT-0", Type: licenses.Permissive}}, nil
	}
	return c.Classifier.Identify(path)
}

var _ licenses.Classifier = classifier{}
