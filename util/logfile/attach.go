package logfile

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/sirupsen/logrus"
)

// ParseLogFileSpecs parses a list of raw spec strings into LogFileSpecs.
// Entries with value "none" are filtered out.
func ParseLogFileSpecs(raw []string, ts time.Time) ([]LogFileSpec, error) {
	var specs []LogFileSpec
	for _, r := range raw {
		spec, err := ParseLogFileSpec(r, ts)
		if errors.Is(err, ErrDisabled) {
			continue
		}
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// AttachLogFiles opens log files and attaches hooks to the logger.
// Returns a cleanup function that closes all hooks and files.
func AttachLogFiles(logger *logrus.Logger, specs []LogFileSpec) (func(), error) {
	var hooks []*FileHook
	for _, spec := range specs {
		dir := filepath.Dir(spec.Path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			// Close any already-opened hooks before returning.
			for _, h := range hooks {
				h.Close()
			}
			return nil, err
		}

		f, err := os.Create(spec.Path)
		if err != nil {
			for _, h := range hooks {
				h.Close()
			}
			return nil, err
		}

		h := NewFileHook(f, spec.Level, spec.Format)
		logger.AddHook(h)
		hooks = append(hooks, h)
	}

	cleanup := func() {
		for _, h := range hooks {
			h.Close()
		}
	}
	return cleanup, nil
}

// BuildLogFileFlag returns a CLI flag for --log-file / --log-files.
func BuildLogFileFlag(dest *cli.StringSlice) *cli.StringSliceFlag {
	return &cli.StringSliceFlag{
		Name:    "log-file",
		Aliases: []string{"log-files"},
		Usage:   "file logging spec: [level=LEVEL;][format=FORMAT;]path=PATH",
		EnvVars: []string{"BLDR_LOG_FILE"},
		Destination: dest,
	}
}
