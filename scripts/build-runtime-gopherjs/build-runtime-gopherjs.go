package main

import (
	"go/build"
	"os"
	"path"
	"path/filepath"

	gbuild "github.com/gopherjs/gopherjs/build"
)

// BuildOptions are the GopherJS build options.
var BuildOptions = gbuild.Options{
	CreateMapFile: true,
	Verbose:       true,
	// Minify: true,
	Color: true,
	BuildTags: []string{
		"js",
		"gopherjs",
		"purego",
	},
}

func execBuild() error {
	build.Default.GOOS = "linux"

	// make tmp gopath
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}

	opts := BuildOptions
	os.Stderr.WriteString("Starting gopherjs build...\n")
	sess, err := gbuild.NewSession(&opts)
	if err != nil {
		return err
	}

	repoRoot := filepath.Join(workDir, "../../")
	runtimeDir := path.Join(repoRoot, "target/browser")
	runtimeOut := path.Join(runtimeDir, "runtime-js.js")
	return sess.BuildDir(
		runtimeDir,
		"main",
		runtimeOut,
	)
}

func main() {
	err := execBuild()
	if err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
		return
	}
	os.Stdout.WriteString("built runtime with gopherjs\n")
}
