package main

import (
	"os"
	"path"
	"path/filepath"

	entrypoint_electron_bundle "github.com/aperturerobotics/bldr/entrypoint/electron/bundle"
	"github.com/sirupsen/logrus"
)

// minify indicates components should be minified
const minify = false

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

	buildDir := path.Join(repoRoot, "build/electron")
	if _, err := os.Stat(buildDir); !os.IsNotExist(err) {
		err = os.RemoveAll(buildDir)
		if err != nil {
			return err
		}
	}

	return entrypoint_electron_bundle.BuildBrowserBundle(le, repoRoot, buildDir, minify)
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
