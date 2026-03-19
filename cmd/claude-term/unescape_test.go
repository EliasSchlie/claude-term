package main

import "testing"

// Prevents: `claude-term write t1 "npm test\n"` sending literal \n instead of newline
func TestUnescapeInput(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`hello\n`, "hello\n"},
		{`hello\r`, "hello\r"},
		{`hello\t`, "hello\t"},
		{`hello\\world`, "hello\\world"},
		{`line1\nline2\n`, "line1\nline2\n"},
		{`no escapes`, "no escapes"},
		{`\n`, "\n"},
		{`trailing backslash\`, "trailing backslash\\"},
		{`unknown \z escape`, "unknown \\z escape"},
		{`\x1b[31m`, "\x1b[31m"},          // ANSI escape
		{`\x1b[0m`, "\x1b[0m"},            // ANSI reset
		{`bad hex \xZZ`, "bad hex \\xZZ"}, // invalid hex passthrough
	}

	for _, tt := range tests {
		got := unescapeInput(tt.input)
		if got != tt.want {
			t.Errorf("unescapeInput(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
