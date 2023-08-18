package web_pkg

import "testing"

func TestValidateWebPkgId(t *testing.T) {
	tests := []struct {
		id          string
		expectedErr error
	}{
		{"", ErrEmptyPkgID},
		{"valid-package", nil},
		{"invalid+package@", ErrInvalidPkgID},
		{"@scope/package", nil},
		{"@scope..package", ErrInvalidPkgID},
		{"some-package", nil},
		{"example.com", nil},
		{"under_score", nil},
		{"period.js", nil},
		{"123numeric", nil},
		{"@npm/thingy", nil},
		{"crazy!", ErrInvalidPkgID},
		{"@npm-zors/money!time.js", ErrInvalidPkgID},
		{".start-with-period", ErrInvalidPkgID},
		{"_start-with-underscore", ErrInvalidPkgID},
		{"contain:colons", ErrInvalidPkgID},
		{" leading-space", ErrInvalidPkgID},
		{"trailing-space ", ErrInvalidPkgID},
		{"s/l/a/s/h/e/s", ErrInvalidPkgID},
		{"CAPITAL-LETTERS", ErrInvalidPkgID},
	}

	for _, test := range tests {
		err := ValidateWebPkgId(test.id)
		if err != test.expectedErr {
			t.Errorf("Expected error for ID %s: %v, got: %v", test.id, test.expectedErr, err)
		}
	}
}

func TestCheckStripWebPkgIdPrefix(t *testing.T) {
	tests := []struct {
		pkgPath     string
		pkgId       string
		pkgSubPath  string
		expectedErr error
	}{
		{"", "", "", ErrEmptyPkgID},
		{"valid-package", "valid-package", "", nil},
		{"valid-package/foo/bar.js", "valid-package", "foo/bar.js", nil},
		{"invalid+package@/foo/bar", "", "", ErrInvalidPkgID},
		{"invalid+package@/foo/bar@/baz", "", "", ErrInvalidPkgID},
		{"@scope/package", "@scope/package", "", nil},
		{"@scope/package/test.js", "@scope/package", "test.js", nil},
		{"@scope/package/foo/bar/baz/test.js", "@scope/package", "foo/bar/baz/test.js", nil},
		{"@scope..package", "", "", ErrInvalidPkgID},
	}
	for _, test := range tests {
		pkgID, pkgSubPath, err := CheckStripWebPkgIdPrefix(test.pkgPath)
		if err != test.expectedErr {
			t.Errorf("Expected error for ID %s: %v, got: %v", test.pkgPath, test.expectedErr, err)
		}
		if err == nil {
			if test.pkgId != pkgID {
				t.Errorf("Expected for ID %s: pkgid %v, got: %v", test.pkgPath, test.pkgId, pkgID)
			}
			if test.pkgSubPath != pkgSubPath {
				t.Errorf("Expected for ID %s: pkgsubpath %v, got: %v", test.pkgPath, test.pkgSubPath, pkgSubPath)
			}
		}
	}
}
