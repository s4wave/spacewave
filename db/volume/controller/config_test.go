package volume_controller

import (
	"testing"
	"time"
)

func TestParseGCIntervalDur(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		want     time.Duration
		disabled bool
	}{
		{
			name: "empty defaults",
			want: defaultGCInterval,
		},
		{
			name:     "bare zero disables gc",
			raw:      "0",
			want:     0,
			disabled: true,
		},
		{
			name:     "duration zero disables gc",
			raw:      "0s",
			want:     0,
			disabled: true,
		},
		{
			name: "duration config",
			raw:  "30s",
			want: 30 * time.Second,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conf := &Config{GcIntervalDur: test.raw}
			got, err := conf.ParseGCIntervalDur()
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("ParseGCIntervalDur() = %s, want %s", got, test.want)
			}
			if conf.GCDisabled() != test.disabled {
				t.Fatalf("GCDisabled() = %v, want %v", conf.GCDisabled(), test.disabled)
			}
			if err := conf.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestParseGCIntervalDurRejectsInvalidDuration(t *testing.T) {
	conf := &Config{GcIntervalDur: "invalid"}
	if _, err := conf.ParseGCIntervalDur(); err == nil {
		t.Fatal("ParseGCIntervalDur() error = nil, want invalid duration error")
	}
	if err := conf.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid duration error")
	}
}
