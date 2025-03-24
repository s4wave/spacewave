package bldr_manifest

import (
	"strings"
	"testing"

	"github.com/pkg/errors"
)

// TestValidateManifestID tests the ValidateManifestID function.
func TestValidateManifestID(t *testing.T) {
	cases := []struct {
		id         string
		allowEmpty bool
		expectErr  bool
		errMsg     string
	}{
		// Valid cases
		{"valid-id", false, false, ""},
		{"valid-id-123", false, false, ""},
		{"valid-id-with-dash", false, false, ""},
		{"v", false, false, ""},
		{"123", false, false, ""},
		{"", true, false, ""}, // Empty is allowed when allowEmpty is true

		// Invalid cases
		{"invalid-id-with-dash-suffix-", false, true, "manifest_id: a DNS-1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character"},
		{"", false, true, "manifest id cannot be empty"},
		{"invalid_id", false, true, "manifest_id: a DNS-1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character"},
		{"Invalid", false, true, "manifest_id: a DNS-1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character"},
		{"too-long-manifest-id-that-exceeds-the-maximum-length-allowed-for-dns-labels", false, true, "manifest_id: length 75 cannot be greater than 63"},
		{"-invalid", false, true, "manifest_id: a DNS-1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character"},
		{"invalid-", false, true, "manifest_id: a DNS-1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character"},
		{"invalid.with.periods", false, true, "manifest_id: a DNS-1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character"},
	}

	for _, tc := range cases {
		err := ValidateManifestID(tc.id, tc.allowEmpty)
		if tc.expectErr {
			if err == nil {
				t.Errorf("case %q: expected error but got none", tc.id)
			} else if tc.errMsg != "" && !errors.Is(err, ErrEmptyManifestID) && !strings.Contains(err.Error(), tc.errMsg) {
				t.Errorf("case %q: expected error containing %q but got %q", tc.id, tc.errMsg, err.Error())
			}
		} else {
			if err != nil {
				t.Errorf("case %q: expected no error but got %q", tc.id, err.Error())
			}
		}
	}
}
