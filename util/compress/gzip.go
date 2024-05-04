package bldr_compress

import (
	"os"
	"path/filepath"
	"time"

	uexec "github.com/aperturerobotics/util/exec"
	"github.com/sirupsen/logrus"
)

// CompressGzip compresses the file using gzip with .br suffix.
func CompressGzip(le *logrus.Entry, workingPath, binPath string) (gzPath string, err error) {
	// track file size savings
	preOptStat, err := os.Stat(binPath)
	if err != nil {
		return "", err
	}
	preOptSize := preOptStat.Size()

	binDir, outBinName := filepath.Dir(binPath), filepath.Base(binPath)
	gzFilename := outBinName + ".gz"
	gzPath = filepath.Join(binDir, gzFilename)

	binPathRel, err := filepath.Rel(workingPath, binPath)
	if err != nil {
		return "", err
	}

	ecmd := uexec.NewCmd(
		"gzip",
		"--best",
		"--keep",
		"--suffix", ".gz",
		binPathRel,
	)
	ecmd.Env = os.Environ()
	ecmd.Dir = workingPath

	timeStart := time.Now()
	if err := uexec.ExecCmd(le, ecmd); err != nil {
		return "", err
	}
	dur := time.Since(timeStart)

	postOptStat, err := os.Stat(gzPath)
	if err != nil {
		return "", err
	}
	postOptSize := postOptStat.Size()

	le.
		WithField("dur", dur.String()).
		Infof("gzip compressed %s from %d -> %d bytes delta %d", gzFilename, preOptSize, postOptSize, postOptSize-preOptSize)
	return gzPath, nil
}
