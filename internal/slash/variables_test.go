package slash

import "testing"

func TestReplaceVariables(t *testing.T) {
	cases := []struct {
		name    string
		content string
		vars    map[string]string
		want    string
	}{
		{
			name:    "skill dir replaced",
			content: "Dir: ${FOXHARNESS_SKILL_DIR}",
			vars:    map[string]string{"FOXHARNESS_SKILL_DIR": "/path/to/skill"},
			want:    "Dir: /path/to/skill",
		},
		{
			name:    "session id replaced",
			content: "Session: ${FOXHARNESS_SESSION_ID}",
			vars:    map[string]string{"FOXHARNESS_SESSION_ID": "uuid-123"},
			want:    "Session: uuid-123",
		},
		{
			name:    "both variables",
			content: "${FOXHARNESS_SKILL_DIR}/${FOXHARNESS_SESSION_ID}",
			vars: map[string]string{
				"FOXHARNESS_SKILL_DIR":  "/p",
				"FOXHARNESS_SESSION_ID": "s",
			},
			want: "/p/s",
		},
		{
			name:    "no variables present",
			content: "static content",
			vars:    map[string]string{"FOXHARNESS_SKILL_DIR": "/p"},
			want:    "static content",
		},
		{
			name:    "empty value replaces with empty",
			content: "Dir: ${FOXHARNESS_SKILL_DIR}",
			vars:    map[string]string{"FOXHARNESS_SKILL_DIR": ""},
			want:    "Dir: ",
		},
		{
			name:    "nil vars map",
			content: "Dir: ${FOXHARNESS_SKILL_DIR}",
			vars:    nil,
			want:    "Dir: ${FOXHARNESS_SKILL_DIR}",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ReplaceVariables(tc.content, tc.vars)
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}
