package exec

import (
	"bytes"
	"context"
	"os"
	osexec "os/exec"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestStartAndWaitPreservesFullStderr(t *testing.T) {
	if os.Getenv("BLDR_EXEC_TEST_FAIL_HELPER") == "1" {
		os.Stderr.WriteString("Error: real bundler failure message\n")
		os.Stderr.WriteString("    at someFrame (file.mjs:1:2)\n")
		os.Stderr.WriteString("    at processTicksAndRejections (native:7:39)\n")
		os.Exit(1)
	}

	log := logrus.New()
	log.SetOutput(&bytes.Buffer{})
	cmd := osexec.CommandContext(context.Background(), os.Args[0], "-test.run=TestStartAndWaitPreservesFullStderr")
	cmd.Env = append(os.Environ(), "BLDR_EXEC_TEST_FAIL_HELPER=1")

	err := StartAndWait(context.Background(), logrus.NewEntry(log), cmd)
	if err == nil {
		t.Fatal("expected error from failing helper")
	}
	// The real cause is the first stderr line; the upstream last-line heuristic
	// would have dropped it in favor of the trailing processTicksAndRejections frame.
	if !strings.Contains(err.Error(), "real bundler failure message") {
		t.Fatalf("expected error to preserve the top stderr message, got %q", err.Error())
	}
}

func TestStartAndWaitRoutesInheritedStdoutToLogger(t *testing.T) {
	if os.Getenv("BLDR_EXEC_TEST_HELPER") == "1" {
		os.Stdout.WriteString("stdout from helper\n")
		os.Stderr.WriteString("stderr from helper\n")
		return
	}

	var out bytes.Buffer
	log := logrus.New()
	log.SetOutput(&out)
	log.SetLevel(logrus.DebugLevel)
	cmd := osexec.CommandContext(context.Background(), os.Args[0], "-test.run=TestStartAndWaitRoutesInheritedStdoutToLogger")
	cmd.Env = append(os.Environ(), "BLDR_EXEC_TEST_HELPER=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := StartAndWait(context.Background(), logrus.NewEntry(log), cmd); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"stdout from helper", "stderr from helper"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected logger output to contain %q, got %q", want, text)
		}
	}
}
