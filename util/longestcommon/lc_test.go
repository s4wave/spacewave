package longestcommon

import (
	"strings"
	"testing"
)

func doTest(t *testing.T, lines, pre, suf string) {
	strs := []string{}
	if lines != "" {
		strs = strings.Split(lines, "\n")
	}
	p := Prefix(strs...)
	if p != pre {
		t.Fatalf("fail: expected prefix '%s', got '%s'", pre, p)
	}
}

func TestXFix1(t *testing.T) {
	doTest(t, ``, "", "")
}

func TestXFix2(t *testing.T) {
	doTest(t, `single`, "single", "single")
}

func TestXFix3(t *testing.T) {
	doTest(t, "single\ndouble", "", "le")
}

func TestXFix4(t *testing.T) {
	doTest(t, "flower\nflow\nfleet", "fl", "")
}

func TestXFix5(t *testing.T) {
	doTest(t, `My Awesome Album - 01.mp3
My Awesome Album - 11.mp3
My Awesome Album - 03.mp3
My Awesome Album - 04.mp3
My Awesome Album - 05.mp3
My Awesome Album - 06.mp3
My Awesome Album - 07.mp3
My Awesome Album - 08.mp3
My Awesome Album - 09.mp3
My Awesome Album - 10.mp3
My Awesome Album - 11.mp3
My Awesome Album - 12.mp3
My Awesome Album - 13.mp3
My Awesome Album - 14.mp3
My Awesome Album - 15.mp3
My Awesome Album - 16.mp3
My Awesome Album - 17.mp3
My Awesome Album - 18.mp3
My Awesome Album - 19.mp3
My Awesome Album - 20.mp3
My Awesome Album - 21.mp3
My Awesome Album - 22.mp3
My Awesome Album - 23.mp3
My Awesome Album - 24.mp3
My Awesome Album - 25.mp3
My Awesome Album - 26.mp3
My Awesome Album - 27.mp3
My Awesome Album - 28.mp3
My Awesome Album - 29.mp3
My Awesome Album - 30.mp3
My Awesome Album - 31.mp3
My Awesome Album - 32.mp3
My Awesome Album - 33.mp3
My Awesome Album - 34.mp3
My Awesome Album - 35.mp3
My Awesome Album - 36.mp3
My Awesome Album - 37.mp3
My Awesome Album - 38.mp3
My Awesome Album - 39.mp3`, "My Awesome Album - ", ".mp3")
}

func TestTrimPrefix1(t *testing.T) {
	strs := []string{"flower", "flow", "fleet"}
	TrimPrefix(strs...)
	if strs[0] != "ower" {
		t.Fatalf("fail: expected result string to be 'ower', got '%s'", strs[0])
	}
}

func TestTrimPrefix2(t *testing.T) {
	strs := []string{"flower", "tree"}
	TrimPrefix(strs...) //no common prefix
	if strs[0] != "flower" {
		t.Fatalf("fail: expected result string to be 'flower', got '%s'", strs[0])
	}
}
