//go:build !js && !wasip1

// Package gitea_testbed drives an explicitly supplied, disposable Gitea application.
package gitea_testbed

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// WaitReady waits for the real HTTP health check until ctx ends.
//
// A cold start has no fixed duration: a VM boot and a first migration can take
// minutes on a loaded host. The caller ends ctx, with a cause, when the server
// can no longer become ready, such as when its process or VM exits.
func WaitReady(t *testing.T, ctx context.Context, base, logPath string) {
	// Bound each probe so a stalled connection cannot outlast the next tick.
	t.Helper()
	client := &http.Client{Timeout: time.Second}

	// Gitea is an external process whose only readiness signal is its HTTP check.
	ticker := time.NewTicker(time.Second) //gostyle:timer external
	defer ticker.Stop()
	for {
		request, _ := http.NewRequestWithContext(ctx, "GET", base+"/api/healthz", nil)
		response, err := client.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case <-ctx.Done():
			log, _ := os.ReadFile(logPath)
			t.Fatalf("Gitea readiness: %v\n%s", context.Cause(ctx), log)
		case <-ticker.C:
		}
	}
}

// Request uses Gitea's existing JSON API, scoped to a disposable account.
func Request(t *testing.T, ctx context.Context, url, password, method, body string) []byte {
	// Authenticate only the request to this fixture's explicit application origin.
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth("operator", password)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	// Keep fixture API responses bounded even when the application reports failure.
	data, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("Gitea %s %s: status=%d err=%v body=%s", method, url, response.StatusCode, err, data)
	}
	return data
}

// Git runs a real Git operation without placing credentials in its arguments.
func Git(t *testing.T, ctx context.Context, dir, password string, args ...string) string {
	// Supply this fixture's credentials and commit identity only to the subprocess.
	t.Helper()
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
	auth := base64.StdEncoding.EncodeToString([]byte("operator:" + password))
	command.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=3",
		"GIT_CONFIG_KEY_0=http.extraHeader", "GIT_CONFIG_VALUE_0=Authorization: Basic "+auth,
		"GIT_CONFIG_KEY_1=user.name", "GIT_CONFIG_VALUE_1=Spacewave fixture",
		"GIT_CONFIG_KEY_2=user.email", "GIT_CONFIG_VALUE_2=fixture@example.invalid",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}
