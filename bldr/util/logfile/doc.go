// Package logfile provides file-based logging for bldr via logrus hooks.
//
// Each log file destination is described by a spec string that can be
// passed via the --log-file CLI flag or the BLDR_LOG_FILE environment
// variable.
//
// # Spec Syntax
//
// A spec string uses semicolon-separated key=value pairs:
//
//	level=DEBUG;format=json;path=.bldr/logs/{ts}.log
//
// Keys:
//   - level: logrus level threshold (default: DEBUG)
//   - format: "text" or "json" (default: text)
//   - path: file path, supports timestamp templates
//
// A short form with just the path is also accepted:
//
//	.bldr/logs/{ts}.log
//
// The special value "none" disables file logging (useful to override
// dev mode auto-enable via BLDR_LOG_FILE=none).
//
// # Timestamp Templates
//
// Templates in the path are expanded once at process start:
//   - {ts}: YYYYMMDD-HHMMSS (e.g., 20260222-143052)
//   - {YYYY}: 4-digit year
//   - {MM}: 2-digit month (zero-padded)
//   - {DD}: 2-digit day (zero-padded)
//   - {HH}: 2-digit hour (zero-padded, 24h)
//   - {mm}: 2-digit minute (zero-padded)
//   - {ss}: 2-digit second (zero-padded)
//
// # Dev Mode
//
// In dev mode (--build-type dev), file logging is auto-enabled with the
// default spec "level=DEBUG;path=.bldr/logs/{ts}.log". Set
// BLDR_LOG_FILE=none or --log-file none to disable.
package logfile
