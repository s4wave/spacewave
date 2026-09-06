package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-licenses/v2/licenses"
)

// TestClassifier distinguishes MIT-0 from MIT and modified license terms.
func TestClassifier(t *testing.T) {
	base, err := licenses.NewClassifier()
	if err != nil {
		t.Fatal(err)
	}
	c := classifier{base}
	for _, test := range []struct {
		name string
		text string
		want string
	}{
		{"mit-zero", "MIT No Attribution License\n\nCopyright (c) 2026 Example\n\n" + mitZero, "MIT-0"},
		{"whitespace", strings.Join(strings.Fields(mitZero), "\r\n"), "MIT-0"},
		{"mit", strings.Replace(mitZero, "THE SOFTWARE IS PROVIDED", "The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.\n\nTHE SOFTWARE IS PROVIDED", 1), "MIT"},
		{"modified", mitZero + "\nRedistribution is forbidden.\n", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "LICENSE")
			if err := os.WriteFile(path, []byte(test.text), 0o600); err != nil {
				t.Fatal(err)
			}
			found, err := c.Identify(path)
			if err != nil {
				t.Fatal(err)
			}
			if test.want != "" && (len(found) != 1 || found[0].Name != test.want) {
				t.Fatalf("got %v, want %s", found, test.want)
			}
			if test.want == "" {
				for _, license := range found {
					if license.Name == "MIT-0" {
						t.Fatal("modified terms accepted as MIT-0")
					}
				}
			}
		})
	}
}
