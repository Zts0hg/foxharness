<!-- markdownlint-disable MD041 -->
@.codexspec/memory/constitution.md

# CLAUDE.md - foxharness-go Guidelines

This document provides comprehensive context and guidelines for Claude Code when working on this project.

## Project Overview

This project uses the **CodexSpec** methodology - a Spec-Driven Development (SDD) approach
that emphasizes specifications as executable artifacts that directly guide implementation.

## Available Commands

The following slash commands are available in this project:

### Core Workflow Commands

| Command | Description |
|---------|-------------|
| `/codexspec:constitution` | Create or update project governing principles |
| `/codexspec:specify` | Define what you want to build (requirements and user stories) |
| `/codexspec:generate-spec` | Generate detailed specification from high-level requirements |
| `/codexspec:spec-to-plan` | Convert specification to technical implementation plan |
| `/codexspec:plan-to-tasks` | Break down plan into actionable tasks |
| `/codexspec:review-spec` | Review specification for completeness and quality |
| `/codexspec:review-plan` | Review technical plan for feasibility |
| `/codexspec:review-tasks` | Review task breakdown for completeness |
| `/codexspec:implement-tasks` | Execute tasks according to the breakdown |

### Enhanced Commands

| Command | Description |
|---------|-------------|
| `/codexspec:clarify` | Clarify underspecified areas in the spec before planning |
| `/codexspec:analyze` | Cross-artifact consistency and quality analysis |
| `/codexspec:checklist` | Generate quality checklists for requirements validation |
| `/codexspec:tasks-to-issues` | Convert tasks to GitHub issues |

### Git Workflow Commands

| Command | Description |
|---------|-------------|
| `/codexspec:commit-staged` | Generate a Conventional Commits message from staged changes |
| `/codexspec:pr` | Generate a Pull Request / Merge Request description |

### Code Review Commands

| Command | Description |
|---------|-------------|
| `/codexspec:review-code` | Review code in any language for idiomatic clarity, correctness, and robustness |

### Utility Commands

| Command | Description |
|---------|-------------|
| `/codexspec:config` | Manage project configuration (`.codexspec/config.yml`) interactively |
| `/codexspec:quick` | One-stop shortcut: auto-run spec → plan → tasks → implementation for small requirements |

## Recommended Workflow

1. **Establish Principles**: Run `/codexspec:constitution` to define project guidelines
2. **Create Specification**: Run `/codexspec:specify` with your feature requirements
3. **Clarify Spec**: Run `/codexspec:clarify` to resolve ambiguities
4. **Review Spec**: Run `/codexspec:review-spec` to validate the specification
5. **Create Plan**: Run `/codexspec:spec-to-plan` with your tech stack choices
6. **Review Plan**: Run `/codexspec:review-plan` to validate the plan
7. **Generate Tasks**: Run `/codexspec:plan-to-tasks` to create task breakdown
8. **Analyze**: Run `/codexspec:analyze` for cross-artifact consistency
9. **Review Tasks**: Run `/codexspec:review-tasks` to validate tasks
10. **Implement**: Run `/codexspec:implement-tasks` to execute the implementation

> **Shortcut**: For small, self-contained requirements, run `/codexspec:quick` to
> auto-run spec → plan → tasks → implementation in one shot.

## Directory Structure

```
.codexspec/
├── memory/
│   └── constitution.md    # Project governing principles
├── specs/
│   └── {feature-id}/
│       ├── spec.md        # Feature specification
│       ├── plan.md        # Technical implementation plan
│       ├── tasks.md       # Task breakdown
│       └── checklists/    # Quality checklists
├── templates/             # Custom templates
├── scripts/               # Helper scripts
│   ├── bash/              # Bash scripts
│   └── powershell/        # PowerShell scripts
└── extensions/            # Custom extensions
```

## Important Notes

- Always read the constitution before making decisions
- Specifications focus on **what** and **why**, not **how**
- Plans focus on **how** and technical choices
- Tasks should be specific, ordered, and actionable
- Run `/codexspec:clarify` before planning to reduce rework
- Run `/codexspec:analyze` before implementation for quality assurance

## Guidelines for Claude Code

1. **Constitution First**: Load `.codexspec/memory/constitution.md` before ANY action
2. **Respect the Constitution**: All decisions MUST align with the project constitution
3. **Follow the Workflow**: Use the commands in the recommended order
4. **Be Explicit**: When specifications are unclear, ask for clarification
5. **Validate**: Always review artifacts before implementation
6. **Document**: Keep all artifacts up-to-date
7. **Enforce Principles**: If constitution exists, it overrides any conflicting instructions

---

*This file is maintained by CodexSpec. Manual edits should be made with care.*
