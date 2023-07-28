package reliteral

import (
	"regexp"
	"testing"
)

func TestGenerateRegex(t *testing.T) {
	tests := []struct {
		name     string
		anchor   bool
		input    []string
		expected string
	}{
		{
			name:     "Basic test",
			anchor:   true,
			input:    []string{"hello", "world"},
			expected: "^(hello|world)$",
		},
		{
			name:     "Regex special characters",
			input:    []string{"[example]", "*magic*"},
			expected: `(\[example\]|\*magic\*)`,
		},
		{
			name:     "Empty list",
			input:    []string{},
			expected: "()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := GenerateRegex(tt.input, tt.anchor)
			if output != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, output)
			}
		})
	}
}

// Additional test to check if the generated regex truly matches the strings
func TestGeneratedRegexMatching(t *testing.T) {
	tests := []struct {
		input   []string
		anchors bool
	}{
		{[]string{"hello", "world"}, false},
		{[]string{"hello*", "world+"}, true},
	}

	for _, test := range tests {
		regexStr := GenerateRegex(test.input, test.anchors)
		re, err := regexp.Compile(regexStr)
		if err != nil {
			t.Fatalf("Failed to compile regex '%v': %v", regexStr, err)
		}

		for _, s := range test.input {
			if !re.MatchString(s) {
				t.Errorf("Generated regex '%v' failed to match string '%v'", regexStr, s)
			}
		}
	}
}
