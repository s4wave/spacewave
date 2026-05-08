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
