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
	withEnvUnset(t, AutoDefaultEnvVar)
	now := time.Date(2026, 5, 4, 12, 30, 0, 0, time.UTC)
	spec, ok := BuildAutoDefaultSpec("/tmp/storage", now)
	if !ok {
		t.Fatalf("expected auto-default to fire when env unset")
	}
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
	if _, ok := BuildAutoDefaultSpec("/tmp/storage", time.Now()); ok {
		t.Errorf("auto-default should not fire when BLDR_LOG_FILE='' (key still present)")
	}
}

func TestBuildAutoDefaultSpec_EmptyRoot(t *testing.T) {
	withEnvUnset(t, AutoDefaultEnvVar)
	if _, ok := BuildAutoDefaultSpec("", time.Now()); ok {
		t.Errorf("auto-default should not fire when storage root is empty")
	}
}

func TestResolveLogLevel(t *testing.T) {
	const (
		spacewave = "TEST_SPACEWAVE_LOG_LEVEL"
		bldr      = "TEST_BLDR_LOG_LEVEL"
	)
	chain := []string{spacewave, bldr}

	t.Run("both unset returns fallback", func(t *testing.T) {
		withEnvUnset(t, spacewave)
		withEnvUnset(t, bldr)
		got := ResolveLogLevel(chain, logrus.InfoLevel)
		if got != logrus.InfoLevel {
			t.Errorf("got %v, want InfoLevel", got)
		}
	})

	t.Run("project-prefixed wins over BLDR_", func(t *testing.T) {
		t.Setenv(spacewave, "warn")
		t.Setenv(bldr, "debug")
		got := ResolveLogLevel(chain, logrus.InfoLevel)
		if got != logrus.WarnLevel {
			t.Errorf("got %v, want WarnLevel", got)
		}
	})

	t.Run("falls through to BLDR_ when project unset", func(t *testing.T) {
		withEnvUnset(t, spacewave)
		t.Setenv(bldr, "debug")
		got := ResolveLogLevel(chain, logrus.InfoLevel)
		if got != logrus.DebugLevel {
			t.Errorf("got %v, want DebugLevel", got)
		}
	})

	t.Run("invalid value falls through", func(t *testing.T) {
		t.Setenv(spacewave, "not-a-level")
		t.Setenv(bldr, "warn")
		got := ResolveLogLevel(chain, logrus.InfoLevel)
		if got != logrus.WarnLevel {
			t.Errorf("got %v, want WarnLevel", got)
		}
	})

	t.Run("blank value skipped", func(t *testing.T) {
		t.Setenv(spacewave, "   ")
		t.Setenv(bldr, "error")
		got := ResolveLogLevel(chain, logrus.InfoLevel)
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
