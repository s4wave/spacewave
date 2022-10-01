package plugin_compiler

import (
	"path"
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
	srcModPath := path.Join(testRootDir, "go.mod")
	destModPath := path.Join(testRootDir, "../next/target-module/go.mod")

	mf, err := modfile.Parse(srcModPath, []byte(testModFile), nil)
	// mf, err := parseGoModFile(srcModPath)
	if err != nil {
		t.Fatal(err.Error())
	}
	if mf.Syntax.Name != srcModPath {
		// mf.Syntax.Name == the absolute path to the go.mod file
		t.Fatalf("%s != %s", mf.Syntax.Name, srcModPath)
	}

	if err := relocateGoModFile(mf, destModPath); err != nil {
		t.Fatal(err.Error())
	}

	outb, err := mf.Format()
	if err != nil {
		t.Fatal(err.Error())
	}
	out := string(outb)
	t.Log(out)

	if out != expectedRelocateModFile {
		t.Fatalf("%s != %s", out, expectedRelocateModFile)
	}
}
