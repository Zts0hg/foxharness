package slash

import "testing"

func TestParseArguments(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a b c", []string{"a", "b", "c"}},
		{`"hello world"`, []string{"hello world"}},
		{`a "b c" d`, []string{"a", "b c", "d"}},
		{`  spaced   args  `, []string{"spaced", "args"}},
		{`unterminated "quoted text`, []string{"unterminated", "quoted text"}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := ParseArguments(tc.in)
			if !sliceEqual(got, tc.want) {
				t.Errorf("ParseArguments(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSubstituteArguments_AllPlaceholderTypes(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		args     []string
		argNames []string
		want     string
	}{
		{
			name:    "$ARGUMENTS full",
			content: "Review: $ARGUMENTS",
			args:    []string{"pr-123"},
			want:    "Review: pr-123",
		},
		{
			name:    "$ARGUMENTS joins multiple",
			content: "Args: $ARGUMENTS",
			args:    []string{"a", "b", "c"},
			want:    "Args: a b c",
		},
		{
			name:    "$ARGUMENTS[0] indexed",
			content: "First: $ARGUMENTS[0]",
			args:    []string{"foo", "bar"},
			want:    "First: foo",
		},
		{
			name:    "$0 $1 shorthand",
			content: "File: $0, Message: $1",
			args:    []string{"main.go", "fix"},
			want:    "File: main.go, Message: fix",
		},
		{
			name:     "named placeholders",
			content:  "$file: $message",
			args:     []string{"main.go", "fix"},
			argNames: []string{"file", "message"},
			want:     "main.go: fix",
		},
		{
			name:    "auto-append when no placeholders",
			content: "Please review the code",
			args:    []string{"pr-123"},
			want:    "Please review the code\n\nARGUMENTS: pr-123",
		},
		{
			name:    "no placeholders no args -> unchanged",
			content: "Hello",
			args:    nil,
			want:    "Hello",
		},
		{
			name:    "missing index replaces empty",
			content: "First $0, Second $1, Third $2",
			args:    []string{"only"},
			want:    "First only, Second , Third ",
		},
		{
			name:     "missing named replaces empty",
			content:  "[$file]-[$message]",
			args:     []string{"main.go"},
			argNames: []string{"file", "message"},
			want:     "[main.go]-[]",
		},
		{
			name:    "multi occurrence",
			content: "$0-$0-$0",
			args:    []string{"x"},
			want:    "x-x-x",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SubstituteArguments(tc.content, tc.args, tc.argNames)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProgressiveHint(t *testing.T) {
	cases := []struct {
		name       string
		argNames   []string
		filled     int
		customHint string
		want       string
	}{
		{
			name:     "no args provided",
			argNames: []string{"file", "message"},
			filled:   0,
			want:     "[file] [message]",
		},
		{
			name:     "one filled",
			argNames: []string{"file", "message", "branch"},
			filled:   1,
			want:     "[message] [branch]",
		},
		{
			name:     "all filled",
			argNames: []string{"file", "message"},
			filled:   2,
			want:     "",
		},
		{
			name:       "custom hint overrides",
			argNames:   []string{"file"},
			customHint: "[choose a file]",
			want:       "[choose a file]",
		},
		{
			name: "no args defined",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ProgressiveHint(tc.argNames, tc.filled, tc.customHint)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
