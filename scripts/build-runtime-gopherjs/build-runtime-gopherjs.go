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
	Minify:        true,
	Color:         true,
	BuildTags: []string{
		"js",
		"linux",
		"gopherjs",
		"purego",
	},
}

func execBuild() error {
	build.Default.GOOS = "js"
	// build.Default.GOARCH = ""

	// make tmp gopath
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}
	repoRoot := filepath.Join(workDir, "../../")
	/*
		tmpGopath := path.Join(workDir, "_gopath")
		if err := os.MkdirAll(tmpGopath, 0755); err != nil {
			return err
		}
	*/

	opts := BuildOptions
	// opts.GOPATH = tmpGopath
	// build.Default.GOPATH = tmpGopath
	// os.Setenv("GOPATH", tmpGopath)
	os.Setenv("GOOS", "linux")
	os.Stderr.WriteString("Starting gopherjs build...\n")
	sess, err := gbuild.NewSession(&opts)
	if err != nil {
		return err
	}
	runtimeDir := path.Join(repoRoot, "runtime")
	return sess.BuildDir(
		runtimeDir,
		"main",
		path.Join(runtimeDir, "runtime.js"),
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
