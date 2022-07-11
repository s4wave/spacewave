package main

import (
	"os"
	"path"
	"path/filepath"

	entrypoint_browser_bundle "github.com/aperturerobotics/bldr/entrypoint/browser/bundle"
	"github.com/sirupsen/logrus"
)

// minify indicates components should be minified
const minify = true

func execBuild(le *logrus.Entry) error {
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}

	repoRoot := filepath.Join(workDir, "../../")
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		return err
	}

	targetDir := path.Join(repoRoot, "target/browser")
	buildDir := path.Join(targetDir, "build")
	if _, err := os.Stat(buildDir); !os.IsNotExist(err) {
		err = os.RemoveAll(buildDir)
		if err != nil {
			return err
		}
	}

	return entrypoint_browser_bundle.BuildBrowserBundle(le, repoRoot, buildDir, minify)
}

func main() {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	err := execBuild(le)
	if err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
		return
	}
}
