// Package effort defines provider-protocol reasoning effort rules.
package effort

import (
	"fmt"
	"strings"
)

const (
	// Auto means foxharness should send no explicit provider effort value.
	Auto = "auto"

	// ProtocolOpenAI selects the OpenAI-compatible effort value set.
	ProtocolOpenAI = "openai"
	// ProtocolClaude selects the Claude-compatible effort value set.
	ProtocolClaude = "claude"
)

var protocolOptions = map[string][]string{
	ProtocolOpenAI: {Auto, "none", "minimal", "low", "medium", "high", "xhigh"},
	ProtocolClaude: {Auto, "low", "medium", "high", "xhigh", "max"},
}

// ResolutionInput contains all sources that participate in user-run effort
// precedence. Earlier non-empty fields have higher priority.
type ResolutionInput struct {
	Protocol    string
	Frontmatter string
	Override    string
	Persisted   string
}

// OptionsForProtocol returns the legal selector values for a provider
// protocol in display order.
func OptionsForProtocol(protocol string) ([]string, error) {
	protocol = normalize(protocol)
	options, ok := protocolOptions[protocol]
	if !ok {
		return nil, fmt.Errorf("unsupported provider protocol %q", protocol)
	}
	return append([]string(nil), options...), nil
}

// Validate normalizes and validates an effort value for a provider protocol.
// An empty value is treated as Auto.
func Validate(protocol string, value string) (string, error) {
	protocol = normalize(protocol)
	value = normalize(value)
	if value == "" {
		value = Auto
	}
	options, ok := protocolOptions[protocol]
	if !ok {
		return "", fmt.Errorf("unsupported provider protocol %q", protocol)
	}
	for _, option := range options {
		if value == option {
			return value, nil
		}
	}
	return "", fmt.Errorf("invalid effort %q for protocol %q", value, protocol)
}

// ExplicitForProvider returns the explicit provider value to send. Auto and an
// empty value return an empty string so callers omit provider effort fields.
func ExplicitForProvider(protocol string, value string) (string, error) {
	value, err := Validate(protocol, value)
	if err != nil {
		return "", err
	}
	if value == Auto {
		return "", nil
	}
	return value, nil
}

// Resolve applies confirmed user-run effort precedence:
// frontmatter, session override, persisted protocol preference, then Auto.
func Resolve(input ResolutionInput) (string, error) {
	for _, value := range []string{input.Frontmatter, input.Override, input.Persisted} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		return Validate(input.Protocol, value)
	}
	return Validate(input.Protocol, Auto)
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
