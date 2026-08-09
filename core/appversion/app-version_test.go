package appversion

import "testing"

func TestGetVersion(t *testing.T) {
	original := versionText
	t.Cleanup(func() { versionText = original })
	for _, test := range []struct {
		name string
		text string
		want string
	}{
		{name: "trimmed", text: " 1.2.3\n", want: "1.2.3"},
		{name: "empty", text: " \n\t", want: "0.0.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			versionText = test.text
			if got := GetVersion(); got != test.want {
				t.Fatalf("GetVersion() = %q, want %q", got, test.want)
			}
		})
	}
}
