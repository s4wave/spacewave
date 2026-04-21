package saucer

import (
	"strings"
	"testing"
)

func TestIsExpression(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{"document.title", true},
		{"1+1", true},
		{"location.href", true},
		{"document.querySelectorAll('.tab').length", true},

		// Statements should not be detected as expressions.
		{"var x = 1", false},
		{"let x = 1", false},
		{"const x = 1", false},
		{"if (true) {}", false},
		{"for (;;) {}", false},
		{"return 1", false},
		{"", false},

		// Multi-line and semicolons are not expressions.
		{"a;\nb", false},
		{"a; b", false},
		{"a\nb", false},

		// Block and comment prefixes.
		{"{ x: 1 }", false},
		{"// comment", false},
		{"/* comment */", false},
	}
	for _, tt := range tests {
		got := isExpression(tt.code)
		if got != tt.want {
			t.Errorf("isExpression(%q) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestWrapEvalCode(t *testing.T) {
	// Expression: should be wrapped with return.
	wrapped := wrapEvalCode("document.title")
	if !strings.Contains(wrapped, "return (document.title)") {
		t.Errorf("expression not wrapped with return: %s", wrapped)
	}

	// Statement: should not be wrapped with return.
	wrapped = wrapEvalCode("var x = 1; return x")
	if strings.Contains(wrapped, "return (var") {
		t.Errorf("statement incorrectly wrapped with return: %s", wrapped)
	}
	if !strings.Contains(wrapped, "var x = 1; return x") {
		t.Errorf("statement code not preserved: %s", wrapped)
	}

	// All wrapped code should contain the __EVAL_ID__ placeholder.
	wrapped = wrapEvalCode("1+1")
	if !strings.Contains(wrapped, "__EVAL_ID__") {
		t.Errorf("missing __EVAL_ID__ placeholder: %s", wrapped)
	}

	// All wrapped code should post the result via postMessage.
	if !strings.Contains(wrapped, "postMessage") {
		t.Errorf("missing postMessage: %s", wrapped)
	}

	// All wrapped code should contain the __bldr_eval marker.
	if !strings.Contains(wrapped, "__bldr_eval") {
		t.Errorf("missing __bldr_eval marker: %s", wrapped)
	}
}
