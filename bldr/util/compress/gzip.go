package bldr_compress

import (
	"context"
	"os"
	"path/filepath"
	"time"

	uexec "github.com/aperturerobotics/util/exec"
	"github.com/sirupsen/logrus"
)

// CompressGzip compresses the file using gzip with .br suffix.
func CompressGzip(ctx context.Context, le *logrus.Entry, workingPath, binPath string) (gzPath string, err error) {
	// track file size savings
	preOptStat, err := os.Stat(binPath)
	if err != nil {
		return "", err
	}
	preOptSize := preOptStat.Size()

	// Derive the gzip output path beside the input binary.
	binDir, outBinName := filepath.Dir(binPath), filepath.Base(binPath)
	gzFilename := outBinName + ".gz"
	gzPath = filepath.Join(binDir, gzFilename)

	// Convert the input path to the compressor working directory.
	binPathRel, err := filepath.Rel(workingPath, binPath)
	if err != nil {
		return "", err
	}

	// Configure gzip to preserve the original binary and write a .gz copy.
	ecmd := uexec.NewCmd(
		ctx,
		"gzip",
		"--best",
		"--keep",
		"--suffix", ".gz",
		binPathRel,
	)
	ecmd.Env = os.Environ()
	ecmd.Dir = workingPath

	// Run gzip and measure the compression time.
	timeStart := time.Now()
	if err := uexec.ExecCmd(le, ecmd); err != nil {
		return "", err
	}
	dur := time.Since(timeStart)

	// Read the compressed output size for the savings report.
	postOptStat, err := os.Stat(gzPath)
	if err != nil {
		return "", err
	}
	postOptSize := postOptStat.Size()

	// Report the compression duration and file-size change.
	le.
		WithField("dur", dur.String()).
		Infof("gzip compressed %s from %d -> %d bytes delta %d", gzFilename, preOptSize, postOptSize, postOptSize-preOptSize)
	return gzPath, nil
}
