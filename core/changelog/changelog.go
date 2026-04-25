package changelog

import _ "embed"

//go:generate go run ../../cmd/changelog-gen --repo ../..

//go:embed changelog.bin
var changelogBinary []byte

// GetChangelog returns the embedded changelog.
func GetChangelog() (*Changelog, error) {
	cl := &Changelog{}
	if err := cl.UnmarshalVT(changelogBinary); err != nil {
		return nil, err
	}
	return cl, nil
}
