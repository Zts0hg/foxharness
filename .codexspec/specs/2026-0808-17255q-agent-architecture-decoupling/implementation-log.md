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

## T040: Feishu Residual Defect Verification

- **Workflow**: Verification-only task. Tests assert controlled current behavior; no production behavior was changed and no desired correction semantics were inferred.
- **Command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -run 'TestDVFEI' -count=1`
- **Result**: PASS, with all ten risks classified as proven defects against production source commit `cdaa566`.
- **DV-FEI-001**: The direct resolver has no authenticated externally reachable HTTP/event route; the candidate callback receives 404 with no mux match.
- **DV-FEI-002**: Sequential, concurrent, post-completion, and post-restart duplicates all receive success acknowledgements and enqueue work again.
- **DV-FEI-003**: Missing sender identity is accepted as empty and causes separate events in one chat to share one persisted session lookup key.
- **DV-FEI-004**: Session-lock waiting cannot observe cancellation and starts previously expired work after the holder releases the mutex.
- **DV-FEI-005**: A same-session lock waiter consumes global capacity, so one conversation can prevent an unrelated conversation from starting; the mutex supplies no FIFO contract.
- **DV-FEI-006**: Runner cancellation and task-channel closure return before accepted in-flight work drains, and `cmd/feishu` has no coordinated signal/intake/server/task shutdown path.
- **DV-FEI-007**: A duplicate approval resolution can block, later report success, and violate exactly-once terminal-state semantics before pending cleanup.
- **DV-FEI-008**: Feishu does not populate the selected model in compactor config, so a known 200K provider model uses the 128K fallback.
- **DV-FEI-009**: Task panic recovery logs and releases capacity but emits no correlated terminal failure reply.
- **DV-FEI-010**: Delivery failures are either logged by Reporter or discarded by Runner; no controlling adapter receives a terminal delivery-failure outcome.
- **Gate outcome**: Correction stop is active. T041 and all later baseline/refactor tasks remain blocked until correction semantics are separately confirmed, each correction follows Red-Green-Refactor in an independent defect commit, and affected tests are rerun.

## T074 / D-FEI-001: Authenticated Approval Callback

- **Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu -run '^TestDVFEI001' -count=1`
- **Red result**: The unauthenticated callback request returned 404 instead of 401 because `/webhook/approval` did not exist.
- **Green implementation**: Added a gateway-owned POST route with constant-time Bearer-token verification, a 64-KiB body limit, strict single-object JSON decoding, required approval identity, shared Store resolution, and deterministic 204/400/401/404/405 mapping. Duplicate-conflict mapping remains assigned to T080.
- **Green commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu ./internal/approval -count=1` and `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/architecturetest -count=1`.
- **Green result**: PASS; `DV-FEI-001` is corrected.

## T075 / D-FEI-002: Durable Message Acceptance

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu -run '^TestDVFEI002' -count=1`
- **Compile Red result**: Expected missing `NewFileDeliveryStore` and `WithDeliveryStore` APIs.
- **Behavior Red command**: The same command after adding the file authority without wiring it into event acceptance.
- **Behavior Red result**: A sequential duplicate still appeared on the task channel, proving the Gateway bypassed the new authority.
- **Green implementation**: Added memory and versioned file `DeliveryStore` implementations, atomic sorted file replacement with restrictive permissions, first-delivery reservation, successful duplicate acknowledgement, cancellation rollback, fail-closed corruption handling, and explicit production composition under the supplied user home.
- **Green commands**: Feishu and cmd package suites, focused delivery-store tests, race tests for concurrent reservation and duplicate dispatch, and the architecture test.
- **Green result**: PASS; `DV-FEI-002` is corrected.

## T076 / D-FEI-003: Sender Identity Validation

- **Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu -run '^TestDVFEI003' -count=1`
- **Red result**: Missing and blank sender events both produced empty-identity tasks and each reached delivery reservation.
- **Green implementation**: Trim and require sender `open_id` during event translation, before task ID generation, durable reservation, session lookup, or enqueue. The Gateway continues to log the invalid event and return Feishu's successful acknowledgement to avoid retry amplification.
- **Green commands**: Feishu and cmd package suites plus the architecture test.
- **Green result**: PASS; `DV-FEI-003` is corrected.

## T077 / D-FEI-004: Acceptance-Scoped Timeout and Cancellable Session Waiting

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu -run '^TestDVFEI004' -count=1`
- **Compile Red result**: The desired cancellation scenario could not compile because `acquireSessionLock` accepted no context and returned no error.
- **Behavior Red command**: The same focused command after introducing a cancellable permit boundary with an intentionally incomplete cancellation result.
- **Behavior Red result**: The cancelled waiter returned a nil error instead of `context.Canceled`, proving the test observed the required terminal cancellation semantics rather than only the new signature.
- **Acceptance-timeout Red result**: `TestDVFEI004AcceptedTaskTimeoutIncludesGlobalPermitWait` could not compile because Runner had no accepted-task context boundary.
- **Green implementation**: Runner now creates the timeout context when it accepts each task, uses it while waiting for global capacity and during execution, rejects an already-expired permit winner, and uses a context-selectable per-session permit whose cancelled waiters release their references without later execution.
- **Green commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1` and `env GOCACHE=/tmp/fox-go-build-cache go test -race ./internal/feishu -run 'TestDVFEI004|TestRunnerSessionLocksCleanupOnlyInactiveEntries' -count=1`.
- **Green result**: PASS; `DV-FEI-004` is corrected. Per-session FIFO/global fairness and coordinated drain remain isolated to T078 and T079.
