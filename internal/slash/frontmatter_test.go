package slash

import (
	"strings"
	"testing"
)

func TestParseFrontmatter_AllFields(t *testing.T) {
	input := []byte(`---
description: "Review code"
arguments: "file message"
argument-hint: "[file] [message]"
allowed-tools:
  - read_file
  - bash
model: "example-model"
effort: "high"
user-invocable: true
disable-model-invocation: false
when_to_use: "Use when reviewing"
context: "fork"
agent: "general-purpose"
paths:
  - "src/**/*.go"
aliases:
  - "r"
  - "rev"
hooks:
  before: "echo start"
  after: "echo done"
version: "1.0"
---
Body content here.
Second line.`)

	fm, body, err := ParseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Description != "Review code" {
		t.Errorf("Description = %q", fm.Description)
	}
	if fm.Arguments != "file message" {
		t.Errorf("Arguments = %q", fm.Arguments)
	}
	if fm.ArgumentHint != "[file] [message]" {
		t.Errorf("ArgumentHint = %q", fm.ArgumentHint)
	}
	if len(fm.AllowedTools) != 2 || fm.AllowedTools[0] != "read_file" || fm.AllowedTools[1] != "bash" {
		t.Errorf("AllowedTools = %v", fm.AllowedTools)
	}
	if fm.Model != "example-model" {
		t.Errorf("Model = %q", fm.Model)
	}
	if fm.Effort != "high" {
		t.Errorf("Effort = %q", fm.Effort)
	}
	if !fm.UserInvocable {
		t.Error("UserInvocable should be true")
	}
	if fm.DisableModelInvocation {
		t.Error("DisableModelInvocation should be false")
	}
	if fm.Context != "fork" {
		t.Errorf("Context = %q", fm.Context)
	}
	if fm.Agent != "general-purpose" {
		t.Errorf("Agent = %q", fm.Agent)
	}
	if len(fm.Paths) != 1 || fm.Paths[0] != "src/**/*.go" {
		t.Errorf("Paths = %v", fm.Paths)
	}
	if len(fm.Aliases) != 2 {
		t.Errorf("Aliases = %v", fm.Aliases)
	}
	if fm.Hooks == nil || fm.Hooks.Before != "echo start" || fm.Hooks.After != "echo done" {
		t.Errorf("Hooks = %+v", fm.Hooks)
	}
	if fm.Version != "1.0" {
		t.Errorf("Version = %q", fm.Version)
	}
	if strings.TrimSpace(body) != "Body content here.\nSecond line." {
		t.Errorf("body = %q", body)
	}
}

func TestParseFrontmatter_AllowedToolsString(t *testing.T) {
	input := []byte(`---
description: "Review code"
allowed-tools: Read, Grep, Bash(git status:*), Bash(git diff:*)
---
Body`)

	fm, body, err := ParseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"read_file", "grep", "bash"}
	if !sliceEqual(fm.AllowedTools, want) {
		t.Fatalf("AllowedTools = %v, want %v", fm.AllowedTools, want)
	}
	if strings.TrimSpace(body) != "Body" {
		t.Fatalf("body = %q", body)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	input := []byte("Just body content\nwith no frontmatter")
	fm, body, err := ParseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fm.UserInvocable {
		t.Error("default UserInvocable should be true")
	}
	if body != "Just body content\nwith no frontmatter" {
		t.Errorf("body = %q", body)
	}
}

func TestParseFrontmatter_InvalidYAML(t *testing.T) {
	input := []byte(`---
description: "valid"
allowed-tools: [unclosed
---
body`)
	fm, body, err := ParseFrontmatter(input)
	if err == nil {
		t.Error("expected non-nil error for invalid YAML")
	}
	if !fm.UserInvocable {
		t.Error("defaults should still apply on parse failure")
	}
	if !strings.Contains(body, "body") {
		t.Errorf("body should be preserved on invalid YAML, got %q", body)
	}
}

func TestParseFrontmatter_Empty(t *testing.T) {
	fm, body, err := ParseFrontmatter([]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fm.UserInvocable {
		t.Error("defaults should apply for empty input")
	}
	if body != "" {
		t.Errorf("expected empty body, got %q", body)
	}
}

func TestParseFrontmatter_MissingClosingDelimiter(t *testing.T) {
	input := []byte(`---
description: "no close"
body content here`)
	fm, body, err := ParseFrontmatter(input)
	if err == nil {
		t.Error("expected warning error for missing closing delimiter")
	}
	if fm.Description != "" {
		t.Errorf("Description should be empty when frontmatter not parsed, got %q", fm.Description)
	}
	if !strings.Contains(body, "body content here") {
		t.Errorf("body should be preserved, got %q", body)
	}
}

func TestParseFrontmatter_OnlyDescription(t *testing.T) {
	input := []byte(`---
description: "just one field"
---
some body`)
	fm, body, err := ParseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Description != "just one field" {
		t.Errorf("Description = %q", fm.Description)
	}
	if !fm.UserInvocable {
		t.Error("unset UserInvocable should default to true")
	}
	if strings.TrimSpace(body) != "some body" {
		t.Errorf("body = %q", body)
	}
}

func TestParseFrontmatter_ExplicitUserInvocableFalse(t *testing.T) {
	input := []byte(`---
user-invocable: false
---
body`)
	fm, _, err := ParseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.UserInvocable {
		t.Error("explicit false must be honored, not overridden by default")
	}
}

func TestParseFrontmatter_MultiLineBodyPreserved(t *testing.T) {
	input := []byte(`---
description: "x"
---
Line 1
Line 2

Line 4 after blank`)
	_, body, err := ParseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "Line 1\nLine 2\n\nLine 4 after blank"
	if strings.TrimSpace(body) != expected {
		t.Errorf("body = %q, want %q", body, expected)
	}
}
