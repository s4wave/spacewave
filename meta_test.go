package bldr

import (
	"strings"
	"testing"
)

// TestGetLicense tests returning the LICENSE file.
func TestGetLicense(t *testing.T) {
	licenseStr := GetLicense()
	t.Log(licenseStr)
	if !strings.Contains(licenseStr, "Aperture Robotics, LLC.") {
		t.Fail()
	}
}
