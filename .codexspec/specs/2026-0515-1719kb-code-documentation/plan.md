# Implementation Plan: Comprehensive Code Documentation

## 1. Tech Stack

| Category | Technology | Version | Notes |
|----------|------------|---------|-------|
| Language | Go | 1.25.0 | Existing project version |
| Documentation | godoc | (built-in) | Standard Go documentation tool |
| Linting | golint | (optional) | For verifying documentation coverage |
| Build Tool | go build | (built-in) | Standard Go toolchain |

## 2. Constitutionality Review

| Principle | Compliance | Notes |
|-----------|------------|-------|
| Principle 1 (TDD) | ✅ | Documentation changes don't require new tests; existing tests must pass |
| Principle 2 (Code Quality) | ✅ | Documentation supports readability through clear explanations |
| Principle 3 (Go Documentation Standards) | ✅ | Core focus: block-level comments only, no teaching comments |
| Principle 4 (Testing Standards) | ✅ | No test logic changes; only documentation additions |
| Principle 5 (Architecture) | ✅ | Public API documentation improves stability and usability |
| Principle 6 (Performance) | ✅ | Documentation has no runtime performance impact |
| Principle 7 (Security) | ✅ | No security implications; documentation-only changes |

## 3. Architecture Overview

This is a documentation-only initiative. The codebase structure remains unchanged. The task involves adding godoc-compatible block comments to:

1. **Package declarations** - Explaining each package's purpose
2. **Exported types** - Documenting struct purposes and field semantics
3. **Exported functions** - Describing behavior, parameters, and return values
4. **Exported methods** - Explaining receiver roles and method behavior
5. **Exported constants/variables** - Documenting their purpose and usage

```
                    ┌─────────────────────────────────────┐
                    │      Documentation Strategy         │
                    └─────────────────────────────────────┘
                                      │
            ┌─────────────────────────┼─────────────────────────┐
            │                         │                         │
            ▼                         ▼                         ▼
    ┌──────────────┐        ┌──────────────┐        ┌──────────────┐
    │ Entry Points │        │ Core Modules │        │   Support    │
    │   (cmd/)     │        │ (internal/)  │        │  Modules     │
    └──────────────┘        └──────────────┘        └──────────────┘
         │                        │                        │
         ▼                        ▼                        ▼
   Package docs             Package docs             Package docs
   Exported funcs           Exported funcs           Exported funcs
   Exported types           Exported types           Exported types
```

## 4. Component Structure

```
foxharness-go/
├── cmd/                           # Entry Points
│   ├── agentops/main.go          # Package: main - AgentOps server
│   ├── bench/main.go             # Package: main - Benchmark runner
│   ├── feishu/main.go            # Package: main - Feishu gateway
│   └── fox/main.go               # Package: main - Main CLI
│
└── internal/                      # Internal Packages
    ├── agentops/                  # Incident analysis service
    ├── app/                       # CLI application layer
    ├── approval/                  # Approval workflows
    ├── benchmark/                 # Benchmark framework
    ├── compaction/                # Context compaction
    ├── context/                   # Prompt context management
    ├── engine/                    # Core agent loop
    ├── feishu/                    # Feishu/Lark integration
    ├── memory/                    # Memory and planning
    ├── metrics/                   # Performance metrics
    ├── middleware/                # Tool middleware
    ├── provider/                  # LLM provider abstraction
    ├── recovery/                  # Error recovery
    ├── reminder/                  # System reminders
    ├── schema/                    # Message schemas
    ├── session/                   # Session management
    ├── subagent/                  # Subagent management
    ├── tools/                     # Tool implementations
    └── tracing/                   # Execution tracing
```

## 5. Module Dependency Graph

```
                    ┌─────────────┐
                    │   engine/   │
                    │  (核心循环)  │
                    └──────┬──────┘
                           │
           ┌───────────────┼───────────────┐
           │               │               │
           ▼               ▼               ▼
    ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
    │  provider/  │ │   tools/    │ │  session/   │
    │ (LLM接口)   │ │ (工具注册)   │ │ (会话管理)   │
    └─────────────┘ └──────┬──────┘ └─────────────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
              ▼            ▼            ▼
       ┌───────────┐ ┌──────────┐ ┌──────────┐
       │  schema/  │ │middleware│ │ recovery/│
       │ (消息模式) │ │(工具中间件)│ │(错误恢复) │
       └───────────┘ └──────────┘ └──────────┘
```

## 6. Module Specifications

### Priority 1: Core Foundation (Highest Impact)

#### Module: internal/engine
- **Responsibility**: Core agent execution loop with turn-based reasoning
- **Dependencies**: provider/, tools/, session/, compaction/, recovery/, reminder/
- **Interface**: Loop struct with Run() method, Config struct
- **Files**:
  - `loop.go` - Add package doc, Loop type doc, Run method doc
  - `config.go` - Add Config type doc, all exported fields

#### Module: internal/provider
- **Responsibility**: LLM provider abstraction and OpenAI-compatible implementation
- **Dependencies**: schema/
- **Interface**: Provider interface, OpenAIProvider implementation
- **Files**:
  - `interface.go` - Add package doc, Provider interface doc
  - `openai.go` - Add OpenAIProvider type and method docs

#### Module: internal/tools
- **Responsibility**: Tool registration and built-in tool implementations
- **Dependencies**: schema/, middleware/
- **Interface**: Registry interface, BaseTool interface
- **Files**:
  - `registry.go` - Add package doc, Registry/Tool interfaces
  - `bash.go`, `read_file.go`, `write_file.go`, `edit_file.go` - Add tool docs

### Priority 2: Session & State Management

#### Module: internal/session
- **Responsibility**: Session lifecycle with persisted memory, transcript, metrics
- **Dependencies**: schema/, tracing/, metrics/
- **Interface**: Manager interface, Session struct
- **Files**:
  - `session.go` - Package doc, Manager interface
  - `memory.go` - Memory operations
  - `transcript.go` - Transcript persistence

#### Module: internal/memory
- **Responsibility**: Plan mode for pre-task planning and memory storage
- **Dependencies**: schema/, provider/
- **Interface**: PlanGenerator interface, Store interface
- **Files**:
  - `plan.go` - Package doc, PlanGenerator
  - `store.go` - Memory store operations

### Priority 3: Supporting Infrastructure

#### Module: internal/compaction
- **Responsibility**: Context history summarization when approaching token limits
- **Dependencies**: provider/, schema/
- **Interface**: Compactor interface
- **Files**: `compactor.go`

#### Module: internal/metrics
- **Responsibility**: Performance metrics recording and aggregation
- **Dependencies**: schema/
- **Interface**: Recorder, Aggregator
- **Files**: `recorder.go`, `summary.go`, `token.go`

#### Module: internal/tracing
- **Responsibility**: Span-based tracing for debugging
- **Dependencies**: None
- **Interface**: Tracer, Load function
- **Files**: `tracer.go`, `reader.go`

#### Module: internal/recovery
- **Responsibility**: Tool failure tracking and recovery prompt injection
- **Dependencies**: schema/
- **Interface**: ErrorTracker
- **Files**: `error_tracker.go`

### Priority 4: Integration Services

#### Module: internal/agentops
- **Responsibility**: Feishu integration for production incident analysis
- **Dependencies**: provider/, feishu/, approval/
- **Interface**: Runner
- **Files**: `runner.go`, `task.go`, `log_search.go`, `prompt.go`

#### Module: internal/feishu
- **Responsibility**: Feishu/Lark webhook gateway and messaging
- **Dependencies**: approval/
- **Interface**: Gateway, Messenger
- **Files**: `gateway.go`, `messenger.go`, `runner.go`, `task.go`

#### Module: internal/approval
- **Responsibility**: Approval workflows for dangerous operations
- **Dependencies**: None
- **Interface**: Store, Approver
- **Files**: `approval.go`, `feishu.go`

### Priority 5: Supporting Modules

#### Module: internal/benchmark
- **Responsibility**: Benchmark framework for validating agent behavior
- **Dependencies**: engine/, session/
- **Interface**: Runner, Case loader
- **Files**: `runner.go`, `case.go`, `validate.go`, `report.go`

#### Module: internal/subagent
- **Responsibility**: Subagent management for parallel task execution
- **Dependencies**: tools/, schema/
- **Interface**: Manager
- **Files**: `manager.go`, `tool.go`

#### Module: internal/middleware
- **Responsibility**: Tool middleware for approval and safety
- **Dependencies**: schema/
- **Interface**: Middleware, DangerMiddleware
- **Files**: `interface.go`, `danger.go`

#### Module: internal/reminder
- **Responsibility**: System reminder injection
- **Dependencies**: schema/
- **Interface**: Reminder
- **Files**: `reminder.go`

#### Module: internal/schema
- **Responsibility**: Message and data structure schemas
- **Dependencies**: None
- **Interface**: Message, ToolCall, ToolResult types
- **Files**: `message.go`

#### Module: internal/context
- **Responsibility**: Prompt context management
- **Dependencies**: schema/
- **Interface**: ContextBuilder
- **Files**: `prompt.go`

#### Module: internal/app
- **Responsibility**: CLI application layer
- **Dependencies**: engine/, provider/, tools/
- **Interface**: CLIConfig, Run function
- **Files**: `cli.go`

### Priority 6: Entry Points

#### Module: cmd/fox
- **Package**: main
- **Responsibility**: Main CLI agent entry point
- **Files**: `main.go` - Add package doc

#### Module: cmd/agentops
- **Package**: main
- **Responsibility**: AgentOps server entry point
- **Files**: `main.go`

#### Module: cmd/feishu
- **Package**: main
- **Responsibility**: Feishu webhook gateway entry point
- **Files**: `main.go`

#### Module: cmd/bench
- **Package**: main
- **Responsibility**: Benchmark runner entry point
- **Files**: `main.go`

## 7. Implementation Phases

### Phase 1: Foundation (Core Engine & Provider)
**Goal**: Document the foundational components that other modules depend on.

- [ ] **internal/engine/loop.go**
  - Add package doc for engine package
  - Document Loop type with its purpose
  - Document Run method with parameters and return values
  - Document any exported methods

- [ ] **internal/engine/config.go**
  - Document Config type
  - Document exported config fields with semantics

- [ ] **internal/provider/interface.go**
  - Add package doc explaining provider abstraction
  - Document Provider interface methods
  - Document any related types

- [ ] **internal/provider/openai.go**
  - Document OpenAIProvider type
  - Document exported methods

### Phase 2: Core Infrastructure (Tools, Session, Memory)
**Goal**: Document core infrastructure components.

- [ ] **internal/tools/registry.go**
  - Add package doc for tools package
  - Document Registry interface
  - Document BaseTool and ParallelSafeTool interfaces
  - Document any exported functions

- [ ] **internal/tools/bash.go**
  - Document BashTool type
  - Document exported methods

- [ ] **internal/tools/read_file.go**
  - Document ReadFileTool type
  - Document exported methods

- [ ] **internal/tools/write_file.go**
  - Document WriteFileTool type
  - Document exported methods

- [ ] **internal/tools/edit_file.go**
  - Document EditFileTool type
  - Document exported methods

- [ ] **internal/session/session.go**
  - Add package doc for session package
  - Document Manager interface
  - Document Session type

- [ ] **internal/session/memory.go**
  - Document Memory operations

- [ ] **internal/session/transcript.go**
  - Document Transcript operations

- [ ] **internal/memory/plan.go**
  - Add package doc for memory package
  - Document PlanGenerator interface

- [ ] **internal/memory/store.go**
  - Document Store operations

### Phase 3: Supporting Systems (Metrics, Tracing, Recovery)
**Goal**: Document supporting infrastructure.

- [ ] **internal/metrics/recorder.go**
  - Add package doc for metrics package
  - Document Recorder type
  - Document EventType constants

- [ ] **internal/metrics/summary.go**
  - Document Aggregator type
  - Document exported methods

- [ ] **internal/metrics/token.go**
  - Document TokenEstimator interface
  - Document RoughEstimator type

- [ ] **internal/tracing/tracer.go**
  - Add package doc for tracing package
  - Document Tracer type
  - Document SpanEvent type
  - Document EventType constants

- [ ] **internal/tracing/reader.go**
  - Document Load function

- [ ] **internal/compaction/compactor.go**
  - Add package doc for compaction package
  - Document Compactor interface
  - Document implementation types

- [ ] **internal/recovery/error_tracker.go**
  - Add package doc for recovery package
  - Document ErrorTracker type

### Phase 4: Integration Services (AgentOps, Feishu, Approval)
**Goal**: Document integration services.

- [ ] **internal/agentops/runner.go**
  - Add package doc for agentops package
  - Document Runner type

- [ ] **internal/agentops/task.go**
  - Document task-related types and functions

- [ ] **internal/agentops/log_search.go**
  - Document log search functionality

- [ ] **internal/agentops/prompt.go**
  - Document prompt templates

- [ ] **internal/feishu/gateway.go**
  - Add package doc for feishu package
  - Document Gateway type

- [ ] **internal/feishu/messenger.go**
  - Document Messenger type

- [ ] **internal/feishu/runner.go**
  - Document Feishu runner

- [ ] **internal/feishu/task.go**
  - Document Feishu task types

- [ ] **internal/approval/approval.go**
  - Add package doc for approval package
  - Document Store interface

- [ ] **internal/approval/feishu.go**
  - Document Feishu approval integration

### Phase 5: Supporting Modules
**Goal**: Document remaining supporting modules.

- [ ] **internal/benchmark/runner.go**
  - Add package doc for benchmark package
  - Document Runner type

- [ ] **internal/benchmark/case.go**
  - Document Case type and LoadCase function

- [ ] **internal/benchmark/validate.go**
  - Document validation functions

- [ ] **internal/benchmark/report.go**
  - Document report generation functions

- [ ] **internal/subagent/manager.go**
  - Add package doc for subagent package
  - Document Manager type

- [ ] **internal/subagent/tool.go**
  - Document SubagentTool type

- [ ] **internal/middleware/interface.go**
  - Add package doc for middleware package
  - Document Middleware interface
  - Document Decision type

- [ ] **internal/middleware/danger.go**
  - Document DangerMiddleware type
  - Document ApprovalRequest type

- [ ] **internal/reminder/reminder.go**
  - Add package doc for reminder package
  - Document Reminder type

- [ ] **internal/schema/message.go**
  - Add package doc for schema package
  - Document Message type
  - Document ToolCall, ToolResult types

- [ ] **internal/context/prompt.go**
  - Add package doc for context package
  - Document context building functions

- [ ] **internal/app/cli.go**
  - Add package doc for app package
  - Document CLIConfig type
  - Document Run function

### Phase 6: Entry Points & Verification
**Goal**: Document CLI entry points and verify completeness.

- [ ] **cmd/fox/main.go**
  - Add package doc for main package
  - Document main function behavior

- [ ] **cmd/agentops/main.go**
  - Add package doc
  - Document environment variable requirements

- [ ] **cmd/feishu/main.go**
  - Add package doc
  - Document environment variable requirements

- [ ] **cmd/bench/main.go**
  - Add package doc
  - Document CLI flags

- [ ] **Verification**
  - Run `go doc ./...` and verify output for all packages
  - Run `gofmt -l .` - ensure no formatting changes needed
  - Run `go test ./...` - ensure all tests pass
  - Code review for teaching comments (none should exist)

## 8. Technical Decisions

### Decision 1: Documentation Location Strategy
- **Choice**: Package documentation in doc.go files for multi-file packages, otherwise before package declaration
- **Rationale**: Follows Go conventions; doc.go files are clearer for packages with multiple files
- **Alternatives**: All package docs before package declaration
- **Trade-offs**: Additional doc.go files vs. consistency in file placement

### Decision 2: Field Documentation Style
- **Choice**: Document struct fields with inline comments for exported fields
- **Rationale**: godoc displays these alongside type documentation; improves API clarity
- **Alternatives**: Only document types, not individual fields
- **Trade-offs**: Slightly more verbose documentation vs. clearer API semantics

### Decision 3: Comment Format for Methods
- **Choice**: Use the full comment format with parameter and return value descriptions
- **Rationale**: Provides complete godoc output; follows Go best practices
- **Alternatives**: Minimal comments without parameter descriptions
- **Trade-offs**: More verbose vs. more complete documentation

### Decision 4: Interface Documentation Placement
- **Choice**: Document interfaces in their definition file, not where they're implemented
- **Rationale**: Interface consumers need the documentation at the interface definition
- **Alternatives**: Duplicate docs at implementation sites
- **Trade-offs**: Potential duplication vs. clearer separation of interface and implementation

## 9. Quality Assurance

### Pre-commit Checklist
- [ ] All package docs follow godoc format
- [ ] All exported identifiers have doc comments
- [ ] No line-level teaching comments exist
- [ ] Code passes `go vet`
- [ ] Code is formatted with `gofmt`
- [ ] All tests pass

### Verification Commands
```bash
# Verify package documentation exists
go doc ./...

# Check formatting
gofmt -l .

# Run tests
go test ./...

# Verify no obvious issues
go vet ./...
```
