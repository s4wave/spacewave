package main

import (
	"context"
	"os"
	"path"
	"path/filepath"

	"github.com/aperturerobotics/controllerbus/util/exec"
	"github.com/sirupsen/logrus"
)

func execBuild() error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// make tmp gopath
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}

	repoRoot := filepath.Join(workDir, "../../")
	runtimeDir := path.Join(repoRoot, "target/browser")
	runtimeOut := path.Join(runtimeDir, "runtime.wasm")
	if _, err := os.Stat(runtimeOut); !os.IsNotExist(err) {
		err = os.Remove(runtimeOut)
		if err != nil {
			return err
		}
	}

	// go version
	_ = exec.StartAndWait(ctx, le, exec.ExecGoCompiler("version"))

	os.Stderr.WriteString("Starting go wasm build...\n")
	ecmd := exec.ExecGoCompiler("build", "-v", "-ldflags", "-s -w", "-o", runtimeOut)
	ecmd.Env = append(ecmd.Env, "GOOS=js", "GOARCH=wasm")
	ecmd.Dir = runtimeDir
	return exec.StartAndWait(ctx, le, ecmd)
}

func main() {
	err := execBuild()
	if err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
		return
	}
	os.Stdout.WriteString("built runtime with wasm\n")
}
