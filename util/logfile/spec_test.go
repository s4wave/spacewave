package logfile

import (
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestParseLogFileSpec(t *testing.T) {
	ts := time.Date(2026, 2, 22, 14, 30, 52, 0, time.UTC)

	tests := []struct {
		name      string
		spec      string
		wantLevel logrus.Level
		wantFmt   string
		wantPath  string
		wantErr   bool
		wantNone  bool
	}{
		{
			name:      "full spec",
			spec:      "level=DEBUG;format=json;path=./logs/{ts}.log",
			wantLevel: logrus.DebugLevel,
			wantFmt:   "json",
			wantPath:  "./logs/20260222-143052.log",
		},
		{
			name:      "short form",
			spec:      "./logs/debug.log",
			wantLevel: logrus.DebugLevel,
			wantFmt:   "text",
			wantPath:  "./logs/debug.log",
		},
		{
			name:      "level only",
			spec:      "level=WARN;path=./warn.log",
			wantLevel: logrus.WarnLevel,
			wantFmt:   "text",
			wantPath:  "./warn.log",
		},
		{
			name:      "format only",
			spec:      "format=json;path=./json.log",
			wantLevel: logrus.DebugLevel,
			wantFmt:   "json",
			wantPath:  "./json.log",
		},
		{
			name:      "template expansion",
			spec:      "path=.bldr/logs/{YYYY}/{MM}/{DD}/{ts}.log",
			wantLevel: logrus.DebugLevel,
			wantFmt:   "text",
			wantPath:  ".bldr/logs/2026/02/22/20260222-143052.log",
		},
		{
			name:      "short form with template",
			spec:      ".bldr/logs/{ts}.log",
			wantLevel: logrus.DebugLevel,
			wantFmt:   "text",
			wantPath:  ".bldr/logs/20260222-143052.log",
		},
		{
			name:      "case insensitive level",
			spec:      "level=info;path=./info.log",
			wantLevel: logrus.InfoLevel,
			wantFmt:   "text",
			wantPath:  "./info.log",
		},
		{
			name:    "invalid level",
			spec:    "level=INVALID;path=./foo.log",
			wantErr: true,
		},
		{
			name:    "invalid format",
			spec:    "format=xml;path=./foo.log",
			wantErr: true,
		},
		{
			name:    "missing path",
			spec:    "level=DEBUG;format=json",
			wantErr: true,
		},
		{
			name:    "unknown key",
			spec:    "foo=bar;path=./foo.log",
			wantErr: true,
		},
		{
			name:     "none",
			spec:     "none",
			wantNone: true,
		},
		{
			name:    "empty string",
			spec:    "",
			wantErr: true,
		},
		{
			name:      "whitespace trimming",
			spec:      " level=DEBUG ; format=json ; path=./logs/test.log ",
			wantLevel: logrus.DebugLevel,
			wantFmt:   "json",
			wantPath:  "./logs/test.log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLogFileSpec(tt.spec, ts)
			if tt.wantNone {
				if !errors.Is(err, ErrDisabled) {
					t.Errorf("expected ErrDisabled, got %v", err)
				}
				return
			}
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Level != tt.wantLevel {
				t.Errorf("level = %v, want %v", got.Level, tt.wantLevel)
			}
			if got.Format != tt.wantFmt {
				t.Errorf("format = %q, want %q", got.Format, tt.wantFmt)
			}
			if got.Path != tt.wantPath {
				t.Errorf("path = %q, want %q", got.Path, tt.wantPath)
			}
		})
	}
}
