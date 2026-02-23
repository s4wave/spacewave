package logfile

import (
	"testing"
	"time"
)

func TestExpandTemplate(t *testing.T) {
	ts := time.Date(2026, 2, 22, 14, 30, 52, 0, time.UTC)

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "ts shorthand",
			path: ".bldr/logs/{ts}.log",
			want: ".bldr/logs/20260222-143052.log",
		},
		{
			name: "individual components",
			path: "logs/{YYYY}/{MM}/{DD}/{HH}-{mm}-{ss}.log",
			want: "logs/2026/02/22/14-30-52.log",
		},
		{
			name: "mixed ts and components",
			path: "{YYYY}/{ts}.log",
			want: "2026/20260222-143052.log",
		},
		{
			name: "no templates",
			path: "logs/debug.log",
			want: "logs/debug.log",
		},
		{
			name: "multiple ts",
			path: "{ts}/{ts}.log",
			want: "20260222-143052/20260222-143052.log",
		},
		{
			name: "empty string",
			path: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandTemplate(tt.path, ts)
			if got != tt.want {
				t.Errorf("ExpandTemplate(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestExpandTemplateZeroPadding(t *testing.T) {
	ts := time.Date(2026, 1, 5, 3, 7, 9, 0, time.UTC)
	got := ExpandTemplate("{YYYY}-{MM}-{DD}_{HH}{mm}{ss}", ts)
	want := "2026-01-05_030709"
	if got != want {
		t.Errorf("zero-padding: got %q, want %q", got, want)
	}
}
