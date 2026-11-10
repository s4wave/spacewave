package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// updateGolden rewrites testdata/realkeys.golden from testdata/realkeys.txt.
var updateGolden = flag.Bool("update-golden", false, "rewrite testdata/realkeys.golden")

// realKeyClass names a key's parser class without its store id: the kind
// and sub-key name of an object store key, or not-objs.
func realKeyClass(key []byte) string {
	k := objKeyOf(key)
	switch {
	case !k.ok:
		return "not-objs"
	case k.kind == kindBad, k.kind == kindForeign:
		return kindNames[k.kind]
	}
	_, rest, _ := objKey(key)
	name, _, _ := strings.Cut(rest, "/")
	return kindNames[k.kind] + " " + name
}

// TestRealKeysGolden classifies the production keys in testdata/realkeys.txt
// (hex, one per line; not committed) and checks the class counts against
// testdata/realkeys.golden. No key may be bad.
func TestRealKeysGolden(t *testing.T) {
	f, err := os.Open("testdata/realkeys.txt")
	if os.IsNotExist(err) {
		t.Skip("testdata/realkeys.txt absent")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Classify each key, and count the write-back stores.
	counts := map[string]int{}
	stores := map[string]struct{}{}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for n := 1; sc.Scan(); n++ {
		key, err := hex.DecodeString(strings.TrimSpace(sc.Text()))
		if err != nil {
			t.Fatalf("line %d: %v", n, err)
		}
		c := realKeyClass(key)
		counts[c]++
		if c == kindNames[kindBad] {
			t.Errorf("line %d: bad object store key %q", n, key)
		}
		if obj, _, ok := objKey(key); ok {
			stores[obj] = struct{}{}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}

	// Render the summary and compare it with the golden file.
	var lines []string
	for c, n := range counts {
		lines = append(lines, fmt.Sprintf("%-40s %d", c, n))
	}
	sort.Strings(lines)
	lines = append(lines, fmt.Sprintf("%-40s %d", "write-back stores", len(stores)))
	got := strings.Join(lines, "\n") + "\n"
	if *updateGolden {
		if err := os.WriteFile("testdata/realkeys.golden", []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/realkeys.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("classes differ from testdata/realkeys.golden:\ngot:\n%swant:\n%s", got, want)
	}
}
