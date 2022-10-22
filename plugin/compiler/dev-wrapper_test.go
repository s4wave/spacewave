package plugin_compiler

import (
	"strings"
	"testing"
)

// TestGetDevWrapper checks embedding the dev src.
func TestGetDevWrapper(t *testing.T) {
	src, err := GetDevWrapper()
	if err != nil {
		t.Fatal(err.Error())
	}
	if !strings.Contains(src, "package main") {
		t.Fail()
	}
}

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
