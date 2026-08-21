package main

import (
	"github.com/Zts0hg/foxharness/internal/childruntime"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/subagent"
)

type childRunnerFactory func(childruntime.Config) subagent.Runner

type foxConfig struct {
	WorkDir         string
	Prompt          string
	Model           string
	LLM             llmconfig.CLIOverrides
	ResolvedLLM     llmconfig.ResolvedConfig
	EffortOverride  string
	EnableThinking  bool
	MaxTurns        int
	SessionID       string
	ContinueSession bool
	NewSession      bool
	Interactive     bool
	NewChildRunner  childRunnerFactory
}
