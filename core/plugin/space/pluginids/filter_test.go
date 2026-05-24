package pluginids

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestFilterValidDropsCorruptSettingsEntries(t *testing.T) {
	got := FilterValid(
		logrus.NewEntry(logrus.New()),
		[]string{"spacewave-notes", "\b\x02\x1aBbinary-plugin-id", "spacewave-app"},
	)
	want := []string{"spacewave-notes", "spacewave-app"}
	if len(got) != len(want) {
		t.Fatalf("plugin ids = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("plugin ids = %q, want %q", got, want)
		}
	}
}
