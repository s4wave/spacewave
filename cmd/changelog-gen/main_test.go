package main

import "testing"

func TestRepoDirDefault(t *testing.T) {
	got, err := repoDir(nil)
	if err != nil {
		t.Fatalf("repoDir(nil) error = %v", err)
	}
	if got == "" {
		t.Fatal("expected current working directory")
	}
}

func TestRepoDirFlag(t *testing.T) {
	got, err := repoDir([]string{"--repo", "/tmp/alpha"})
	if err != nil {
		t.Fatalf("repoDir(flag) error = %v", err)
	}
	if got != "/tmp/alpha" {
		t.Fatalf("expected /tmp/alpha, got %q", got)
	}
}

func TestRepoDirRejectsInvalidArgs(t *testing.T) {
	if _, err := repoDir([]string{"--bad"}); err == nil {
		t.Fatal("expected invalid args error")
	}
}
