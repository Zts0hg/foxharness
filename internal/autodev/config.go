package autodev

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// GateConfig toggles the completion-gate commands. Test carries a floor:
// it is mandatory and Load forces it back on with a warning when a config
// file tries to disable it (REQ-018).
type GateConfig struct {
	Build bool
	Test  bool
	Gofmt bool
}

// RemoteFlowConfig toggles the remote publishing steps. AutoMerge is always
// false: merging stays a human/CI responsibility (REQ-020).
type RemoteFlowConfig struct {
	CreateIssue bool
	OpenPR      bool
	LinkIssue   bool
	AutoMerge   bool
}

// AutodevConfig is the resolved configuration for one autodev run, loaded
// from .foxharness/autodev.yml with defaults applied for any missing file
// or key (REQ-023).
type AutodevConfig struct {
	BacklogFile        string
	WorktreeDir        string
	BaseBranch         string
	Remote             string
	Concurrency        string
	Model              string
	EngineerPrompt     string
	EngineerPromptFile string
	Pipeline           string
	Gates              GateConfig
	RemoteFlow         RemoteFlowConfig

	// Warnings collects prominent notices produced while resolving the
	// config, e.g. a gate disabled by the user or a forced gate floor.
	Warnings []string
}

// configFile is the YAML wire form. Booleans are pointers so an absent key
// is distinguishable from an explicit false.
type configFile struct {
	BacklogFile        string `yaml:"backlog_file"`
	WorktreeDir        string `yaml:"worktree_dir"`
	BaseBranch         string `yaml:"base_branch"`
	Remote             string `yaml:"remote"`
	Concurrency        string `yaml:"concurrency"`
	Model              string `yaml:"model"`
	EngineerPrompt     string `yaml:"engineer_prompt"`
	EngineerPromptFile string `yaml:"engineer_prompt_file"`
	Pipeline           string `yaml:"pipeline"`
	Gates              struct {
		Build *bool `yaml:"build"`
		Test  *bool `yaml:"test"`
		Gofmt *bool `yaml:"gofmt"`
	} `yaml:"gates"`
	RemoteFlow struct {
		CreateIssue *bool `yaml:"create_issue"`
		OpenPR      *bool `yaml:"open_pr"`
		LinkIssue   *bool `yaml:"link_issue"`
		AutoMerge   *bool `yaml:"auto_merge"`
	} `yaml:"remote_flow"`
}

// Load reads .foxharness/autodev.yml under repoRoot and returns the resolved
// AutodevConfig. A missing file applies all defaults; a present file fills
// only the keys it sets. The test gate cannot be disabled (gate floor), and
// auto_merge can never be enabled (REQ-018, REQ-020).
func Load(repoRoot string) (AutodevConfig, error) {
	cfg := defaultConfig(repoRoot)

	data, err := os.ReadFile(filepath.Join(repoRoot, ".foxharness", "autodev.yml"))
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read autodev.yml: %w", err)
	}

	var file configFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return cfg, fmt.Errorf("parse autodev.yml: %w", err)
	}

	applyString(&cfg.BacklogFile, file.BacklogFile)
	applyString(&cfg.WorktreeDir, file.WorktreeDir)
	applyString(&cfg.BaseBranch, file.BaseBranch)
	applyString(&cfg.Remote, file.Remote)
	applyString(&cfg.Concurrency, file.Concurrency)
	applyString(&cfg.Model, file.Model)
	applyString(&cfg.EngineerPrompt, file.EngineerPrompt)
	applyString(&cfg.EngineerPromptFile, file.EngineerPromptFile)
	applyString(&cfg.Pipeline, file.Pipeline)

	applyBool(&cfg.Gates.Build, file.Gates.Build)
	applyBool(&cfg.Gates.Test, file.Gates.Test)
	applyBool(&cfg.Gates.Gofmt, file.Gates.Gofmt)
	applyBool(&cfg.RemoteFlow.CreateIssue, file.RemoteFlow.CreateIssue)
	applyBool(&cfg.RemoteFlow.OpenPR, file.RemoteFlow.OpenPR)
	applyBool(&cfg.RemoteFlow.LinkIssue, file.RemoteFlow.LinkIssue)

	cfg.enforceFloors(file)
	return cfg, nil
}

// enforceFloors applies the constitution-safe limits after the user config
// is merged: the test gate is forced on, disabled optional gates warn, and
// auto_merge is rejected.
func (c *AutodevConfig) enforceFloors(file configFile) {
	if !c.Gates.Test {
		c.Gates.Test = true
		c.Warnings = append(c.Warnings,
			"WARNING: the test gate is mandatory and cannot be disabled; forcing gates.test back on (REQ-018)")
	}
	if !c.Gates.Build {
		c.Warnings = append(c.Warnings,
			"WARNING: gates.build is disabled; items can pass the completion gate without compiling the whole repo")
	}
	if !c.Gates.Gofmt {
		c.Warnings = append(c.Warnings,
			"WARNING: gates.gofmt is disabled; items can pass the completion gate with unformatted code")
	}
	if file.RemoteFlow.AutoMerge != nil && *file.RemoteFlow.AutoMerge {
		c.Warnings = append(c.Warnings,
			"WARNING: remote_flow.auto_merge is not supported; PRs are never auto-merged (REQ-020)")
	}
	c.RemoteFlow.AutoMerge = false
}

func defaultConfig(repoRoot string) AutodevConfig {
	repoName := filepath.Base(repoRoot)
	return AutodevConfig{
		BacklogFile: "BACKLOG.md",
		WorktreeDir: filepath.Join("..", repoName+"-worktrees"),
		BaseBranch:  "main",
		Remote:      "origin",
		Concurrency: "serial",
		Pipeline:    "lean",
		Gates:       GateConfig{Build: true, Test: true, Gofmt: true},
		RemoteFlow:  RemoteFlowConfig{CreateIssue: true, OpenPR: true, LinkIssue: true, AutoMerge: false},
	}
}

func applyString(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}

func applyBool(dst *bool, v *bool) {
	if v != nil {
		*dst = *v
	}
}
