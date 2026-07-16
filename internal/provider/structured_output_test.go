package provider

import "testing"

type structuredOutputStatusError struct {
	StatusCode int
	message    string
}

func (e structuredOutputStatusError) Error() string { return e.message }

func TestStructuredOutputRejectionRecognizesInvalidValueResponse(t *testing.T) {
	err := structuredOutputStatusError{
		StatusCode: 400,
		message:    "Invalid value: 'json_schema'; supported values are: 'json_object'",
	}
	if !isStructuredOutputRejection(err) {
		t.Fatalf("isStructuredOutputRejection(%q) = false, want true", err.Error())
	}
}
