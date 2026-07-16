package effort

import "testing"

func TestOptionsForProtocol(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		want     []string
	}{
		{
			name:     "openai",
			protocol: ProtocolOpenAI,
			want:     []string{Auto, "none", "minimal", "low", "medium", "high", "xhigh"},
		},
		{
			name:     "claude",
			protocol: ProtocolClaude,
			want:     []string{Auto, "low", "medium", "high", "xhigh", "max"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := OptionsForProtocol(tt.protocol)
			if err != nil {
				t.Fatalf("OptionsForProtocol() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("OptionsForProtocol() len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("OptionsForProtocol()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		protocol  string
		value     string
		wantValue string
		wantErr   bool
	}{
		{name: "openai minimal", protocol: ProtocolOpenAI, value: " minimal ", wantValue: "minimal"},
		{name: "openai none", protocol: ProtocolOpenAI, value: "none", wantValue: "none"},
		{name: "openai max invalid", protocol: ProtocolOpenAI, value: "max", wantErr: true},
		{name: "claude max", protocol: ProtocolClaude, value: "max", wantValue: "max"},
		{name: "claude minimal invalid", protocol: ProtocolClaude, value: "minimal", wantErr: true},
		{name: "auto accepted", protocol: ProtocolClaude, value: Auto, wantValue: Auto},
		{name: "empty defaults to auto", protocol: ProtocolOpenAI, value: "", wantValue: Auto},
		{name: "unsupported protocol", protocol: "other", value: "low", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Validate(tt.protocol, tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Validate() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if got != tt.wantValue {
				t.Fatalf("Validate() = %q, want %q", got, tt.wantValue)
			}
		})
	}
}

func TestExplicitForProvider(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		value    string
		want     string
	}{
		{name: "auto omits", protocol: ProtocolOpenAI, value: Auto, want: ""},
		{name: "empty omits", protocol: ProtocolClaude, value: "", want: ""},
		{name: "explicit returns value", protocol: ProtocolClaude, value: "xhigh", want: "xhigh"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExplicitForProvider(tt.protocol, tt.value)
			if err != nil {
				t.Fatalf("ExplicitForProvider() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ExplicitForProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name        string
		frontmatter string
		override    string
		persisted   string
		want        string
	}{
		{name: "frontmatter wins", frontmatter: "high", override: "low", persisted: "medium", want: "high"},
		{name: "override wins persisted", override: "low", persisted: "medium", want: "low"},
		{name: "persisted wins auto", persisted: "xhigh", want: "xhigh"},
		{name: "empty resolves auto", want: Auto},
		{name: "frontmatter auto clears lower values", frontmatter: Auto, override: "high", persisted: "medium", want: Auto},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(ResolutionInput{
				Protocol:    ProtocolClaude,
				Frontmatter: tt.frontmatter,
				Override:    tt.override,
				Persisted:   tt.persisted,
			})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveRejectsInvalidHigherPrecedenceValue(t *testing.T) {
	_, err := Resolve(ResolutionInput{
		Protocol:    ProtocolClaude,
		Frontmatter: "minimal",
		Override:    "high",
		Persisted:   "low",
	})
	if err == nil {
		t.Fatal("Resolve() error = nil, want invalid frontmatter error")
	}
}
