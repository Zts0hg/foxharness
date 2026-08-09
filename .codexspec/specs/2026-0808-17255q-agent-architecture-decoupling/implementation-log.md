# Implementation Log

## T001: Characterization Trace Authority

- **Workflow**: Documentation task; direct implementation.
- **Outcome**: Added `characterization-trace.md` with one evidence row for each confirmed scenario and defect-verification identifier.
- **Verification**: Extracted stable IDs from `requirements.md` and trace rows, sorted both sets, and compared them with `comm`.
- **Result**: 274 required IDs, 274 traced IDs, empty missing and extra sets.

## T002: Dependency Documentation and Decreasing Allowlist

- **Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/architecturetest -run TestProductionImportsMatchArchitectureAllowlist -count=1`
- **Expected Red**: The architecture test failed because the exact baseline allowlist did not exist. An empty allowlist then failed with 68 current production violations, proving the parser and new-violation gate observed the existing graph.
- **Additional Red**: After adding the current ledger, the same test failed because the immutable baseline ceiling did not exist. This established the missing protection against simultaneous import and allowlist broadening.
- **Green implementation**: Added the AST import scanner, confirmed target rules, 68-edge immutable baseline ceiling, identical initial decreasing ledger, deletion boundaries, and authoritative `docs/package-dependencies.md`.
- **Green command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/architecturetest -count=1`
- **Green result**: PASS. `baseline_allowlist.json` and `allowlist.json` are byte-identical with 68 entries at initialization.
- **Environment note**: The first attempted command used the default macOS Go build cache and failed because that directory is outside the writable sandbox. It is not counted as TDD Red; all recorded test evidence uses the test-owned `/tmp` cache.

## T003: Immutable Fixture Support

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/testsupport/entryfixture -count=1`
- **Compile Red result**: Expected missing API errors for `Manifest`, `Fixture`, `Load`, `CopyFixture`, `SequenceClock`, and `IDSequence`.
- **Behavior Red command**: The same command after adding API-complete no-op scaffolding.
- **Behavior Red result**: Tests failed for undetected hash tampering, accepted incomplete frozen authority, missing independent copy, accepted traversal, and nondeterministic clock/ID results.
- **Green implementation**: Added manifest decoding and validation, SHA-256 integrity checks, contained regular-file resolution, independent copy support, a versioned open manifest, and mutex-protected deterministic clock and ID sequences.
- **Green commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/testsupport/entryfixture -count=1` and `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/architecturetest -count=1`.
- **Green result**: PASS.

## T004: Shared Runtime Contract DSL

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/testsupport/runtimecontract -count=1`
- **Compile Red result**: Expected missing API errors for the adapter, scenario, input, script, observation, fact, outcome, and artifact contracts.
- **Behavior Red command**: The same command after adding API-complete no-op verification scaffolding.
- **Behavior Red result**: Tests failed because reordered facts, changed artifacts, and malformed scenario IDs were accepted.
- **Green implementation**: Added production-API-independent scenario inputs, scripted model/tool/interaction values, adapter contract, ordered facts, outcomes, persisted records, artifacts, warnings, exact comparison, expected adapter-error semantics, and stable ID validation.
- **Green commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/testsupport/runtimecontract -count=1` plus the entry-fixture and architecture-test packages.
- **Green result**: PASS.

## T005: Current Production Contract Adapter

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/engine -run TestCurrentProductionContractAdapterRunsToolFreeScenario -count=1`
- **Compile Red result**: Expected undefined test-adapter constructor; no production compatibility API existed.
- **Behavior Red command**: The same command after adding the test-only adapter.
- **Behavior Red result**: Exact contract comparison rejected an empty warning slice where the authority expected nil, proving the adapter observation was not normalized by the verifier.
- **Green implementation**: Added an unexported `_test.go` adapter using the real `AgentEngine`, an isolated current session manager, a scripted provider, lifecycle reporter, and message-log observation. The tool-free `RT-001` proof checks ordered events, final outcome, turn count, and persisted user/assistant records without adding a production symbol.
- **Green commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/engine -run TestCurrentProductionContractAdapterRunsToolFreeScenario -count=1` and the runtime-contract, entry-fixture, and architecture-test package suites.
- **Green result**: PASS.

## T006: Hermetic Collaborators

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/testsupport/hermetic -count=1`
- **Compile Red result**: Expected missing APIs for barriers, scripted model/tool/interaction boundaries, clocks, IDs, roots, filesystem, process, HTTP, messenger, command, local Git, and fake GitHub collaborators.
- **Behavior Red command**: The same command after implementing the collaborator APIs.
- **Behavior Red result**: The isolated local-Git test observed a macOS temporary-directory environment warning in command output because `TMPDIR` had not been fixed, demonstrating environment-dependent output.
- **Green implementation**: Added finite scripted collaborators with immutable request snapshots, explicit cancellable barriers, controlled streaming and failures, aliases/correlation, fixed clocks and IDs, operation-level filesystem failures, bounded process trees, an in-memory `http.RoundTripper`, fake messenger/command/GitHub boundaries, explicit state roots, and a local-Git repository with isolated HOME, TMPDIR, and Git configuration. A source-policy test rejects external network clients/listeners, ambient HOME and credential access, uncontrolled clock/random sources, and sleep synchronization.
- **Green commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/testsupport/... -count=1` and focused current-adapter plus architecture-test commands.
- **Green result**: PASS.
- **Phase 0A gate**: `env GOCACHE=/tmp/fox-go-build-cache go test ./...` passed for every package when run with local-test networking and test-state writes enabled. The initial sandbox attempt failed only because existing provider tests require an ephemeral `httptest` listener and existing app/benchmark/subagent tests resolve the user HOME outside the writable sandbox; those environment denials are excluded from behavioral evidence.
