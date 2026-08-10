//go:build !darwin

package tools

func newPlatformReadOnlyBashRunner() readOnlyBashRunner {
	return unavailableReadOnlyBashRunner{}
}
