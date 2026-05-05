package storagepath

import "testing"

func TestStorageRootEnvVar(t *testing.T) {
	tests := []struct {
		projectID string
		want      string
	}{
		{"spacewave", "SPACEWAVE_DATA_DIR"},
		{"my-project", "MY_PROJECT_DATA_DIR"},
		{"foo bar", "FOOBAR_DATA_DIR"},
		{"weird!@#name", "WEIRDNAME_DATA_DIR"},
	}
	for _, tt := range tests {
		if got := StorageRootEnvVar(tt.projectID); got != tt.want {
			t.Errorf("StorageRootEnvVar(%q) = %q, want %q", tt.projectID, got, tt.want)
		}
	}
}

func TestLogLevelEnvVar(t *testing.T) {
	tests := []struct {
		projectID string
		want      string
	}{
		{"spacewave", "SPACEWAVE_LOG_LEVEL"},
		{"my-project", "MY_PROJECT_LOG_LEVEL"},
		{"foo bar", "FOOBAR_LOG_LEVEL"},
	}
	for _, tt := range tests {
		if got := LogLevelEnvVar(tt.projectID); got != tt.want {
			t.Errorf("LogLevelEnvVar(%q) = %q, want %q", tt.projectID, got, tt.want)
		}
	}
}

func TestLogRetentionDaysEnvVar(t *testing.T) {
	if got := LogRetentionDaysEnvVar("spacewave"); got != "SPACEWAVE_LOG_RETENTION_DAYS" {
		t.Errorf("LogRetentionDaysEnvVar(spacewave) = %q", got)
	}
	if got := LogRetentionDaysEnvVar("my-project"); got != "MY_PROJECT_LOG_RETENTION_DAYS" {
		t.Errorf("LogRetentionDaysEnvVar(my-project) = %q", got)
	}
}
