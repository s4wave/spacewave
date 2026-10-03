package logfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// withEnvUnset clears key for the duration of the test, restoring it on
// cleanup if it was set originally.
func withEnvUnset(t *testing.T, key string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unsetenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, prev)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func TestBuildAutoDefaultSpec_Unset(t *testing.T) {
	// Resolve a default log destination with explicit configuration absent.
	withEnvUnset(t, AutoDefaultEnvVar)
	now := time.Date(2026, 5, 4, 12, 30, 0, 0, time.UTC)
	spec, ok := BuildAutoDefaultSpec("/tmp/storage", now)
	if !ok {
		t.Fatalf("expected auto-default to fire when env unset")
	}

	// Verify the default log level, format, and expanded timestamp path.
	if spec.Level != logrus.DebugLevel {
		t.Errorf("level = %v, want DEBUG", spec.Level)
	}
	if spec.Format != "text" {
		t.Errorf("format = %q, want text", spec.Format)
	}
	want := filepath.Join("/tmp/storage", "logs", "20260504-123000.log")
	if spec.Path != want {
		t.Errorf("path = %q, want %q", spec.Path, want)
	}
}

func TestBuildAutoDefaultSpec_Set(t *testing.T) {
	t.Setenv(AutoDefaultEnvVar, "level=WARN;path=/tmp/foo.log")
	if _, ok := BuildAutoDefaultSpec("/tmp/storage", time.Now()); ok {
		t.Errorf("auto-default should not fire when BLDR_LOG_FILE is set")
	}
}

func TestBuildAutoDefaultSpec_None(t *testing.T) {
	t.Setenv(AutoDefaultEnvVar, "none")
	if _, ok := BuildAutoDefaultSpec("/tmp/storage", time.Now()); ok {
		t.Errorf("auto-default should not fire when BLDR_LOG_FILE=none")
	}
}

func TestBuildAutoDefaultSpec_EmptyString(t *testing.T) {
	t.Setenv(AutoDefaultEnvVar, "")
	if _, ok := BuildAutoDefaultSpec("/tmp/storage", time.Now()); !ok {
		t.Errorf("auto-default should fire when BLDR_LOG_FILE=''")
	}
}

func TestBuildAutoDefaultSpec_BlankString(t *testing.T) {
	t.Setenv(AutoDefaultEnvVar, "   ")
	if _, ok := BuildAutoDefaultSpec("/tmp/storage", time.Now()); !ok {
		t.Errorf("auto-default should fire when BLDR_LOG_FILE is blank")
	}
}

func TestBuildAutoDefaultSpec_EmptyRoot(t *testing.T) {
	withEnvUnset(t, AutoDefaultEnvVar)
	if _, ok := BuildAutoDefaultSpec("", time.Now()); ok {
		t.Errorf("auto-default should not fire when storage root is empty")
	}
}

func TestResolveLogLevel(t *testing.T) {
	// Define the project and builder log-level precedence.
	const (
		spacewave = "TEST_SPACEWAVE_LOG_LEVEL"
		bldr      = "TEST_BLDR_LOG_LEVEL"
	)
	chain := []string{spacewave, bldr}

	// Verify missing log-level settings use the fallback.
	t.Run("both unset returns fallback", func(t *testing.T) {
		// Clear both log-level settings for the fallback case.
		withEnvUnset(t, spacewave)
		withEnvUnset(t, bldr)

		// Resolve the configured log level.
		got := ResolveLogLevel(chain, logrus.InfoLevel)

		// Verify the resolved level matches the fallback.
		if got != logrus.InfoLevel {
			t.Errorf("got %v, want InfoLevel", got)
		}
	})

	// Verify the project log-level setting takes precedence.
	t.Run("project-prefixed wins over BLDR_", func(t *testing.T) {
		// Configure competing project and builder log levels.
		t.Setenv(spacewave, "warn")
		t.Setenv(bldr, "debug")

		// Resolve the configured log level.
		got := ResolveLogLevel(chain, logrus.InfoLevel)

		// Verify the resolved level matches the project or builder warning setting.
		if got != logrus.WarnLevel {
			t.Errorf("got %v, want WarnLevel", got)
		}
	})

	// Verify the builder setting applies when the project setting is absent.
	t.Run("falls through to BLDR_ when project unset", func(t *testing.T) {
		// Configure only the builder log level.
		withEnvUnset(t, spacewave)
		t.Setenv(bldr, "debug")

		// Resolve the configured log level.
		got := ResolveLogLevel(chain, logrus.InfoLevel)

		// Verify the resolved level matches the builder debug setting.
		if got != logrus.DebugLevel {
			t.Errorf("got %v, want DebugLevel", got)
		}
	})

	// Verify an invalid project setting permits the builder fallback.
	t.Run("invalid value falls through", func(t *testing.T) {
		// Configure an invalid project level and a valid builder level.
		t.Setenv(spacewave, "not-a-level")
		t.Setenv(bldr, "warn")

		// Resolve the configured log level.
		got := ResolveLogLevel(chain, logrus.InfoLevel)

		// Verify the resolved level matches the project or builder warning setting.
		if got != logrus.WarnLevel {
			t.Errorf("got %v, want WarnLevel", got)
		}
	})

	// Verify a blank project setting permits the builder fallback.
	t.Run("blank value skipped", func(t *testing.T) {
		// Configure a blank project level and a valid builder level.
		t.Setenv(spacewave, "   ")
		t.Setenv(bldr, "error")

		// Resolve the configured log level.
		got := ResolveLogLevel(chain, logrus.InfoLevel)

		// Verify the resolved level matches the builder error setting.
		if got != logrus.ErrorLevel {
			t.Errorf("got %v, want ErrorLevel", got)
		}
	})
}

func TestResolveRetention(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantDur  time.Duration
		wantWarn bool
	}{
		{"unset", "", DefaultRetention, false},
		{"blank", "   ", DefaultRetention, false},
		{"valid 1", "1", 24 * time.Hour, false},
		{"valid 14", "14", 14 * 24 * time.Hour, false},
		{"zero", "0", DefaultRetention, false},
		{"negative", "-3", DefaultRetention, false},
		{"non-numeric", "hello", DefaultRetention, true},
		{"trailing units", "7d", DefaultRetention, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dur, warn := ResolveRetention("SPACEWAVE_LOG_RETENTION_DAYS", tt.raw)
			if dur != tt.wantDur {
				t.Errorf("dur = %v, want %v", dur, tt.wantDur)
			}
			if (warn != "") != tt.wantWarn {
				t.Errorf("warn = %q, want non-empty=%v", warn, tt.wantWarn)
			}
		})
	}
}
