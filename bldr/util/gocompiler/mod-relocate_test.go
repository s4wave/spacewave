package gocompiler

import (
	"path/filepath"
	"testing"

	"golang.org/x/mod/modfile"
)

var testRootDir = "/does/not/exist/src-module"

var testModFile = `module github.com/src-module

replace google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55

replace github.com/relative-module => ../relative-module

require (
	github.com/blang/semver v3.5.1+incompatible
)
`

var expectedRelocateModFile = `module github.com/src-module

replace google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55

replace github.com/relative-module => ../../relative-module

require github.com/blang/semver v3.5.1+incompatible
`

// TestRelocateGoModFile tests relocating a sample go.mod file.
func TestRelocateGoModFile(t *testing.T) {
	// Choose the source and destination paths for the module fixture.
	srcModPath := filepath.Join(testRootDir, "go.mod")
	destModPath := filepath.Join(testRootDir, "../next/target-module/go.mod")

	// Parse the source module fixture for relocation.
	mf, err := modfile.Parse(srcModPath, []byte(testModFile), nil)

	// mf, err := parseGoModFile(srcModPath)
	if err != nil {
		t.Fatal(err.Error())
	}
	if mf.Syntax.Name != srcModPath {
		// mf.Syntax.Name == the absolute path to the go.mod file
		t.Fatalf("%s != %s", mf.Syntax.Name, srcModPath)
	}

	// Relocate the module replacements to the destination path.
	if err := RelocateGoModFile(mf, destModPath); err != nil {
		t.Fatal(err.Error())
	}

	// Format the relocated module for comparison with the expected text.
	outb, err := mf.Format()
	if err != nil {
		t.Fatal(err.Error())
	}
	out := string(outb)
	t.Log(out)

	// Verify that the relocated module preserves the expected replacements.
	if out != expectedRelocateModFile {
		t.Fatalf("%s != %s", out, expectedRelocateModFile)
	}
}
