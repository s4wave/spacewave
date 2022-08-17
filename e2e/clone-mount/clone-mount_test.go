package clone_mount

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestCloneMount runs the clone-mount test.
func TestCloneMount(t *testing.T) {
	podmanURL := strings.Join([]string{
		"unix:///run/user/",
		strconv.Itoa(os.Getuid()),
		"/podman/podman.sock",
	}, "")

	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if err := Run(ctx, le, podmanURL); err != nil {
		t.Fatal(err.Error())
	}
}
