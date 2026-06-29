package configcmd

// OnboardingMessage returns the actionable guidance shown when foxharness starts
// with no configured LLM provider. It names `fox config` as the remediation step
// and deliberately avoids assuming any vendor or required environment variable.
func OnboardingMessage() string {
	return "No LLM provider is configured.\n\n" +
		"Add one interactively with:\n" +
		"    fox config\n\n" +
		"Then start fox again."
}
