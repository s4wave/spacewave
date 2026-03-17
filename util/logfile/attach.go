package logfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/mattn/go-isatty"
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

// EnsureLoggerLevel raises the logger level to the most verbose hook level.
// If the logger level is already sufficient, this is a no-op. Otherwise it
// redirects the logger's console output through a level-filtered hook so
// that --log-level controls console verbosity independently of file hooks.
func EnsureLoggerLevel(logger *logrus.Logger, specs []LogFileSpec) {
	maxLevel := logger.GetLevel()
	for _, spec := range specs {
		if spec.Level > maxLevel {
			maxLevel = spec.Level
		}
	}
	if maxLevel <= logger.GetLevel() {
		return
	}

	// Preserve color output: logrus TextFormatter detects TTY via
	// entry.Logger.Out, which will be io.Discard after the swap. Build
	// a new formatter with ForceColors when the original output is a TTY.
	formatter := logger.Formatter
	if tf, ok := formatter.(*logrus.TextFormatter); ok && !tf.DisableColors && writerIsTerminal(logger.Out) {
		formatter = &logrus.TextFormatter{
			ForceColors:               true,
			ForceQuote:                tf.ForceQuote,
			DisableQuote:              tf.DisableQuote,
			EnvironmentOverrideColors: tf.EnvironmentOverrideColors,
			DisableTimestamp:          tf.DisableTimestamp,
			FullTimestamp:             tf.FullTimestamp,
			TimestampFormat:           tf.TimestampFormat,
			DisableSorting:            tf.DisableSorting,
			SortingFunc:               tf.SortingFunc,
			DisableLevelTruncation:    tf.DisableLevelTruncation,
			PadLevelText:              tf.PadLevelText,
			QuoteEmptyFields:          tf.QuoteEmptyFields,
			FieldMap:                  tf.FieldMap,
			CallerPrettyfier:          tf.CallerPrettyfier,
		}
	}

	consoleHook := NewConsoleHook(logger.Out, formatter, logger.GetLevel())
	logger.AddHook(consoleHook)
	logger.SetOutput(io.Discard)
	logger.SetLevel(maxLevel)
}

// writerIsTerminal reports whether w is connected to a terminal.
func writerIsTerminal(w io.Writer) bool {
	if f, ok := w.(interface{ Fd() uintptr }); ok {
		return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
	}
	return false
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
