package provider

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrStructuredOutputUnsupported indicates that an endpoint or model rejected
// the provider-native structured output option.
var ErrStructuredOutputUnsupported = errors.New("provider-native structured output is unsupported")

func normalizeStructuredOutputError(err error) error {
	if err == nil || !isStructuredOutputRejection(err) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrStructuredOutputUnsupported, err)
}

func isStructuredOutputRejection(err error) bool {
	status, ok := errorStatusCode(err)
	if ok && status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
		return false
	}

	message := strings.ToLower(err.Error())
	mentionsOption := containsAny(message,
		"response_format",
		"response format",
		"json_schema",
		"json schema",
		"output_config",
		"output format",
		"structured output",
	)
	if !mentionsOption {
		return false
	}
	if ok {
		return true
	}
	return containsAny(message,
		"not support",
		"unsupported",
		"unknown",
		"unrecognized",
		"unexpected",
		"not permitted",
		"extra input",
		"extra_forbidden",
		"invalid parameter",
		"invalid value",
		"supported values",
		"not available",
		"unavailable",
		"not enabled",
	)
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
