package web_pkg

import (
	"context"
	"path"
	"regexp"
	"strings"

	"github.com/aperturerobotics/hydra/unixfs"
)

// WebPkg is a service serving files for a web package (abbreviated pkg).
// The web package ID is equivalent to the npm package ID (i.e. "@myorg/mypkg").
type WebPkg interface {
	// GetId returns the web package identifier.
	GetId() string
	// GetInfo returns the WebPkgInfo for the WebPkg.
	GetInfo(ctx context.Context) (*WebPkgInfo, error)
	// GetWebPkgFsHandle returns an fs handle which can be used to access the WebPkg fs.
	// Remember to release the handle when done.
	GetWebPkgFsHandle(ctx context.Context) (*unixfs.FSHandle, error)
}

// PkgNameRe is a regex that can be used to check for valid package names.
// https://github.com/dword-design/package-name-regex/blob/2899905/src/index.js
var PkgNameRe = regexp.MustCompile(`^(@[a-z0-9-~][a-z0-9-._~]*\/)?[a-z0-9-~][a-z0-9-._~]*$`)

// ValidateWebPkgId validates a web package identifier.
// Follows the npm package name validation scheme.
func ValidateWebPkgId(id string) error {
	// Check if package ID is empty
	if len(id) == 0 {
		return ErrEmptyPkgID
	}

	// Check if package id is invalid
	if !PkgNameRe.MatchString(id) {
		return ErrInvalidPkgID
	}

	return nil
}

// CheckStripWebPkgIdPrefix checks and strips a web pkg id prefix from a path.
//
// Returns the web pkg id and the pkg path split.
func CheckStripWebPkgIdPrefix(pkgPath string) (pkgID, pkgSubPath string, err error) {
	pkgPath = strings.TrimSpace(pkgPath)
	pkgPath = strings.TrimPrefix(pkgPath, "/")
	if len(pkgPath) == 0 {
		return "", pkgPath, ErrEmptyPkgID
	}

	pkgIdBefore, pkgSubPath, _ := strings.Cut(pkgPath, "/")
	if pkgIdBefore[0] == '@' {
		var pkgIdAfter string
		pkgIdAfter, pkgSubPath, _ = strings.Cut(pkgSubPath, "/")
		pkgID = path.Join(pkgIdBefore, pkgIdAfter)
	} else {
		pkgID = pkgIdBefore
	}

	pkgSubPath = path.Clean(pkgSubPath)
	if len(pkgSubPath) != 0 && (pkgSubPath[0] == '/' || pkgSubPath[0] == '.') {
		pkgSubPath = pkgSubPath[1:]
	}

	return pkgID, pkgSubPath, ValidateWebPkgId(pkgID)
}
