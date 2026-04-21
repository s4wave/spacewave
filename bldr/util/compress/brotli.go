package bldr_compress

import (
	"context"
	"os"
	"path/filepath"
	"time"

	uexec "github.com/aperturerobotics/util/exec"
	"github.com/sirupsen/logrus"
)

// CompressBrotli compresses the file using brotli with .br suffix.
func CompressBrotli(ctx context.Context, le *logrus.Entry, workingPath, binPath string) (brPath string, err error) {
	// track file size savings
	preOptStat, err := os.Stat(binPath)
	if err != nil {
		return "", err
	}
	preOptSize := preOptStat.Size()

	binDir, outBinName := filepath.Dir(binPath), filepath.Base(binPath)
	brFilename := outBinName + ".br"
	brPath = filepath.Join(binDir, brFilename)

	brPathRel, err := filepath.Rel(workingPath, brPath)
	if err != nil {
		return "", err
	}

	binPathRel, err := filepath.Rel(workingPath, binPath)
	if err != nil {
		return "", err
	}

	ecmd := uexec.NewCmd(
		ctx,
		"brotli",
		// Compression levels have a trade-off between build time and file size.
		// -q 11 (--best): 50s, file size: 4.9M
		// -q 9: 1.8s, file size: 5.6M
		// -q 4: 200ms, file size: 6.4M
		// see: https://devblogs.microsoft.com/dotnet/performance_improvements_in_net_7/#compression
		"-q", "9",
		"--keep",
		"-o", brPathRel,
		binPathRel,
	)
	ecmd.Env = os.Environ()
	ecmd.Dir = workingPath

	timeStart := time.Now()
	if err := uexec.ExecCmd(le, ecmd); err != nil {
		return "", err
	}
	dur := time.Since(timeStart)

	postOptStat, err := os.Stat(brPath)
	if err != nil {
		return "", err
	}
	postOptSize := postOptStat.Size()

	le.
		WithField("dur", dur.String()).
		Infof("brotli compressed %s from %d -> %d bytes delta %d", brFilename, preOptSize, postOptSize, postOptSize-preOptSize)
	return brPath, nil
}
