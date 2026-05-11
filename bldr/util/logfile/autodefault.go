package logfile

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// AutoDefaultEnvVar is the environment variable that controls the file
// logging spec. When set to a non-empty value, including "none", the user's
// configuration wins and BuildAutoDefaultSpec returns no spec.
const AutoDefaultEnvVar = "BLDR_LOG_FILE"

// DefaultRetention is the on-disk retention duration applied when no
// project-scoped override resolves to a positive duration.
const DefaultRetention = 7 * 24 * time.Hour

// BuildAutoDefaultSpec returns the auto-default LogFileSpec for an
// entrypoint that has no explicit BLDR_LOG_FILE configuration.
//
// The resolved spec writes DEBUG-level text records to
// <storageRoot>/logs/{ts}.log, where {ts} is expanded against now.
// If BLDR_LOG_FILE is set to a non-empty value (including "none"), the user's
// configuration takes precedence and this function returns ok=false so the
// caller defers to the existing parsing path. Empty values behave like the
// variable is unset and still get the auto-default.
//
// Callers receive the spec but are responsible for opening the file via
// AttachLogFiles and pruning old logs via PruneOldLogs.
func BuildAutoDefaultSpec(storageRoot string, now time.Time) (LogFileSpec, bool) {
	if raw, ok := os.LookupEnv(AutoDefaultEnvVar); ok && strings.TrimSpace(raw) != "" {
		return LogFileSpec{}, false
	}
	if storageRoot == "" {
		return LogFileSpec{}, false
	}
	path := filepath.Join(storageRoot, "logs", "{ts}.log")
	return LogFileSpec{
		Level:  logrus.DebugLevel,
		Format: "text",
		Path:   ExpandTemplate(path, now),
	}, true
}

// ResolveRetention returns the configured on-disk log retention duration.
// It reads the project-scoped retention env var (e.g.
// SPACEWAVE_LOG_RETENTION_DAYS) and falls back to DefaultRetention when
// the variable is unset, blank, non-positive, or unparseable as a
// non-negative integer.
//
// When the variable is set but cannot be parsed, the returned warn message
// is non-empty so callers can emit one logrus.Warn at startup. A
// successful parse always returns warn == "".
func ResolveRetention(env, raw string) (dur time.Duration, warn string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultRetention, ""
	}
	days, err := strconv.Atoi(raw)
	if err != nil {
		return DefaultRetention, env + "=" + strconv.Quote(raw) +
			" is not a non-negative integer; using default retention"
	}
	if days <= 0 {
		return DefaultRetention, ""
	}
	return time.Duration(days) * 24 * time.Hour, ""
}

// ResolveLogLevel walks envChain in order, returning the first parseable
// logrus level found. Empty / unset values are skipped; a value that fails
// to parse is also skipped (callers receive fallback in that case).
// Returns fallback when no env var resolves.
func ResolveLogLevel(envChain []string, fallback logrus.Level) logrus.Level {
	for _, name := range envChain {
		raw := strings.TrimSpace(os.Getenv(name))
		if raw == "" {
			continue
		}
		lvl, err := logrus.ParseLevel(raw)
		if err != nil {
			continue
		}
		return lvl
	}
	return fallback
}

// EnableAutoDefault enables file logging at the storage-root-relative
// default location when BLDR_LOG_FILE is unset. It prunes stale logs,
// attaches the file hook, and raises the logger level so console
// filtering applies via EnsureLoggerLevel.
//
// Returns a cleanup function that closes the file hook (nil when no
// hook was attached). When BLDR_LOG_FILE is set or storageRoot is
// empty, this is a no-op and returns (nil, nil).
//
// retentionEnv names the project-scoped retention env var (e.g.
// "SPACEWAVE_LOG_RETENTION_DAYS"); pass "" to skip the override and
// always use DefaultRetention. logger receives the retention warning
// (if any) before the hook is attached so it lands in stderr but not
// in the about-to-be-created file.
func EnableAutoDefault(
	logger *logrus.Logger,
	storageRoot string,
	retentionEnv string,
	now time.Time,
) (func(), error) {
	spec, ok := BuildAutoDefaultSpec(storageRoot, now)
	if !ok {
		return nil, nil
	}

	retention := DefaultRetention
	if retentionEnv != "" {
		dur, warn := ResolveRetention(retentionEnv, os.Getenv(retentionEnv))
		retention = dur
		if warn != "" {
			logger.Warn(warn)
		}
	}

	logsDir := filepath.Dir(spec.Path)
	if _, err := PruneOldLogs(logsDir, retention, now); err != nil {
		logger.WithError(err).Warn("failed to prune old logs")
	}

	cleanup, err := AttachLogFiles(logger, []LogFileSpec{spec})
	if err != nil {
		return nil, err
	}
	EnsureLoggerLevel(logger, []LogFileSpec{spec})
	return cleanup, nil
}
