package bldr_plugin_compiler_go

import (
	"testing"
)

// TestValidateDlvAddr tests validating the delve address.
func TestValidateDlvAddr(t *testing.T) {
	if err := ValidateDelveAddr("192.168.0.1:8080"); err != nil {
		t.Fail()
	}
	if err := ValidateDelveAddr(":8080"); err != nil {
		t.Fail()
	}
	if err := ValidateDelveAddr("asdf we 2 13\""); err == nil {
		t.Fail()
	}
	if err := ValidateDelveAddr("\"192.168.1.1:8080"); err == nil {
		t.Fail()
	}
}
