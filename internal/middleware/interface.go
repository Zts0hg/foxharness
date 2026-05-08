package middleware

import (
	"context"

	"github.com/Zts0hg/foxharness/internal/schema"
)

type DecisionType string

const (
	DecisionAllow DecisionType = "allow"
	DecisionDeny  DecisionType = "deny"
)

type Decision struct {
	Type   DecisionType
	Reason string
}

type Middleware interface {
	BeforeExecute(ctx context.Context, call schema.ToolCall) (Decision, error)
}

func Allow() Decision {
	return Decision{Type: DecisionAllow}
}

func Deny(reason string) Decision {
	return Decision{Type: DecisionDeny, Reason: reason}
}
