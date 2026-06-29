package configcmd

import (
	"strings"
	"testing"
)

func TestOnboardingMessageGuidesToFoxConfig(t *testing.T) {
	msg := OnboardingMessage()
	if !strings.Contains(msg, "fox config") {
		t.Errorf("OnboardingMessage() = %q, want to mention `fox config`", msg)
	}
	for _, banned := range []string{"ZHIPU", "glm-4.5-air"} {
		if strings.Contains(msg, banned) {
			t.Errorf("OnboardingMessage() = %q, must not assume vendor %q", msg, banned)
		}
	}
}
