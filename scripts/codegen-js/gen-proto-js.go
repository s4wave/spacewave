package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

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
	outPath := path.Join(repoRoot, "src", "bldr", "proto")
	outPathJs := path.Join(outPath, "proto.js")
	outPathTs := path.Join(outPath, "proto.ts")

	binDir := path.Join(repoRoot, "node_modules", ".bin")
	pbjsPath := path.Join(binDir, "pbjs")
	pbtsPath := path.Join(binDir, "pbts")

	_ = pbjsPath
	_ = pbtsPath

	runtimeDir := path.Join(repoRoot, "runtime")
	runtimeDir, err = filepath.Abs(runtimeDir)
	if err != nil {
		return err
	}

	// list files
	cmd := exec.NewCmd("git", "ls-files", "**/*.proto")
	cmd.Dir = runtimeDir
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	err = exec.StartAndWait(ctx, le, cmd)
	if err != nil {
		return err
	}

	fileList := strings.Split(outBuf.String(), "\n")
	for i := 0; i < len(fileList); i++ {
		fileList[i] = strings.TrimSpace(fileList[i])
		if len(fileList[i]) == 0 {
			fileList[i] = fileList[len(fileList)-1]
			fileList = fileList[:len(fileList)-1]
			i--
		}
	}

	os.Stderr.WriteString("Generating js for proto files: ")
	os.Stderr.WriteString(fmt.Sprintf("%#v", fileList) + "\n")

	cmd = exec.NewCmd(
		pbjsPath,
		append([]string{
			"-t", "static-module",
			"-w", "es6", // commonjs
			"-p", repoRoot,
			"-o", outPathJs,
		}, fileList...)...,
	)
	cmd.Dir = runtimeDir
	err = exec.StartAndWait(ctx, le, cmd)
	if err != nil {
		return err
	}

	cmd = exec.NewCmd(
		pbtsPath,
		"-o", outPathTs,
		outPathJs,
	)
	cmd.Dir = runtimeDir
	err = exec.StartAndWait(ctx, le, cmd)
	if err != nil {
		return err
	}

	/*
		for _, fn := range fileList {
			fn = path.Join(runtimeDir, fn)
			if _, err := os.Stat(fn); err != nil {
				return err
			}

			// dir containing the source file
			fd := path.Dir(fn)
			fext := path.Ext(fn)
			// fnb is the base name without .proto
			fname := path.Base(fn)
			fnb := fname[:len(fname)-len(fext)]
			jsn := fnb + ".pb.js"
			jsf := path.Join(fd, jsn)
			os.Stderr.WriteString("generating " + jsf + "\n")
		}
	*/

	return nil
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
