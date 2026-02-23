package logfile

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ErrDisabled is returned by ParseLogFileSpec when the spec is "none".
var ErrDisabled = errors.New("log file disabled")

// LogFileSpec describes a log file destination.
type LogFileSpec struct {
	// Level is the logrus level threshold.
	Level logrus.Level
	// Format is the output format ("text" or "json").
	Format string
	// Path is the expanded file path (templates already resolved).
	Path string
}

// ParseLogFileSpec parses a log file spec string into a LogFileSpec.
//
// Spec syntax (semicolon-delimited key=value pairs):
//   - Full form: level=DEBUG;format=json;path=./.bldr/logs/{ts}.log
//   - Short form: ./.bldr/logs/{ts}.log (path only, defaults apply)
//   - "none" -> returns ErrDisabled
//
// Default level: DEBUG. Default format: text.
// Unknown keys produce an error. Missing path produces an error.
func ParseLogFileSpec(spec string, ts time.Time) (LogFileSpec, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return LogFileSpec{}, errors.New("empty log file spec")
	}
	if spec == "none" {
		return LogFileSpec{}, ErrDisabled
	}

	result := LogFileSpec{
		Level:  logrus.DebugLevel,
		Format: "text",
	}

	// Check if this is a short form (no semicolons and no '=' sign).
	if !strings.Contains(spec, ";") && !strings.Contains(spec, "=") {
		result.Path = ExpandTemplate(spec, ts)
		return result, nil
	}

	fields := strings.Split(spec, ";")
	for i, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}

		before, after, ok := strings.Cut(field, "=")
		if !ok {
			// Last field without '=' is treated as path.
			if i == len(fields)-1 {
				result.Path = ExpandTemplate(field, ts)
				continue
			}
			return LogFileSpec{}, fmt.Errorf("invalid field %q: missing '='", field)
		}

		key := strings.TrimSpace(before)
		val := strings.TrimSpace(after)

		switch key {
		case "level":
			lvl, err := logrus.ParseLevel(val)
			if err != nil {
				return LogFileSpec{}, fmt.Errorf("invalid level %q: %w", val, err)
			}
			result.Level = lvl
		case "format":
			if val != "text" && val != "json" {
				return LogFileSpec{}, fmt.Errorf("invalid format %q: must be \"text\" or \"json\"", val)
			}
			result.Format = val
		case "path":
			result.Path = ExpandTemplate(val, ts)
		default:
			return LogFileSpec{}, fmt.Errorf("unknown key %q", key)
		}
	}

	if result.Path == "" {
		return LogFileSpec{}, errors.New("missing path in log file spec")
	}

	return result, nil
}
