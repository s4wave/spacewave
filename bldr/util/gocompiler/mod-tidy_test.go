package gocompiler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestRunGoModTidyExecutesGoTool(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "go.mod"), []byte("module example.com/tidy\n\ngo 1.26.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	le := logrus.NewEntry(logrus.New())
	if err := RunGoModTidy(context.Background(), le, workDir); err != nil {
		t.Fatal(err)
	}
}
