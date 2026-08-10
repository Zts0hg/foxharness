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
- **Post-correction full-gate Red**: The original request-cancellation fixture timed out because the Lark dispatcher did not propagate the HTTP request context into the event callback, leaving an unavailable unbuffered task queue blocked indefinitely.
- **Follow-up Green**: Commit `28386a0` made unavailable local enqueue fail fast, atomically release the durable reservation, and return an observable handler error. `TestGatewayRollsBackReservationWhenEnqueueIsUnavailable` uses deterministic queue replacement to prove retry success without relying on SDK context propagation.

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

## T078 / D-FEI-005: Per-Session FIFO and Global Scheduling Fairness

- **Behavior Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu -run '^TestDVFEI005' -count=1`
- **Behavior Red result**: `same-2` started while `same-1` was still active, proving that the existing global semaphore admitted same-session waiters before session eligibility.
- **Green implementation**: Added a Runner-private event-loop scheduler with one FIFO queue per `(chat_id, sender_id)`, an ordered ready-session queue, and event-loop-owned active capacity. Only a session head can run globally; queued successors hold no global capacity; unrelated ready sessions retain admission order; completion enables only the completed session's next head. Accepted task contexts remain scoped from enqueue through execution, and expired queue heads are skipped.
- **Green commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1` and `env GOCACHE=/tmp/fox-go-build-cache go test -race ./internal/feishu -run '^TestDVFEI005PerSessionFIFOLeavesCapacityForOtherSessions$' -count=1 -v`.
- **Green result**: PASS; `DV-FEI-005` is corrected. Runner drain/cancel and process shutdown remain isolated to T079.
- **Post-correction test alignment**: The complete package gate correctly rejected the old global-limit test's expectation that two tasks from one session could run concurrently. Commit `3dd89e1` assigns that test's four tasks to distinct sessions so it verifies only the global limit; DV-FEI-005 remains the separate FIFO authority.

## T079 / D-FEI-006: Coordinated Drain, Cancellation, and Process Shutdown

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -run '^TestDVFEI006' -count=1`
- **Compile Red result**: The production-order test could not compile because no `serve` coordination boundary existed.
- **Behavior Red result**: In the same run, ordinary task-channel closure caused `Runner.Start` to return before its accepted task completed.
- **Green implementation**: Runner now stops acceptance and drains queued/active work on ordinary channel closure; process cancellation stops acceptance, explicitly cancels queued and active task contexts, and waits for active completion. The production entry uses `signal.NotifyContext` and a testable `serve` composition that shuts down and confirms the HTTP listener first, then closes task intake, cancels Runner work, and waits for listener/Runner completion within one explicit deadline. If listener termination times out, it does not close the still-reachable task channel.
- **Green commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1` and `env GOCACHE=/tmp/fox-go-build-cache go test -race ./internal/feishu ./cmd/feishu -run '^TestDVFEI006' -count=1 -v`.
- **Green result**: PASS; `DV-FEI-006` is corrected. Approval terminal-state semantics remain isolated to T080.

## T080 / D-FEI-007: Non-Blocking Exactly-Once Approval Resolution

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/approval ./internal/feishu -run 'TestDVFEI007|TestStoreTimeoutRemovesPendingRequest' -count=1`
- **Compile Red result**: The desired tests could not compile because Store exposed neither typed not-found/conflict errors nor an injectable timeout signal.
- **Behavior Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu -run '^TestDVFEI007ApprovalResolutionIsNonBlockingAndExactlyOnce$' -count=1`
- **Behavior Red result**: A duplicate `Resolve` remained blocked behind the first buffered result for the bounded 50-ms observation window.
- **Green implementation**: Replaced result-channel capacity as authority with a mutex-arbitrated pending record. The first resolution atomically stores its result and closes one completion signal; duplicates while claimed return `ErrConflict`; cancellation, timeout, send failure, and resolved Wait completion remove the record so unknown/late attempts return `ErrNotFound`. Resolve-vs-cancel/timeout races linearize under the same mutex. Gateway maps conflict to 409 and not-found to 404 after authentication.
- **Green commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/approval ./internal/feishu ./cmd/feishu -count=1` and `env GOCACHE=/tmp/fox-go-build-cache go test -race ./internal/approval ./internal/feishu -run 'TestStoreConcurrentResolveHasExactlyOneWinner|TestDVFEI007' -count=1 -v`.
- **Green result**: PASS; `DV-FEI-007` is corrected, including deterministic timeout injection and a 32-resolver one-winner race proof.

## T081 / D-FEI-008: Frozen Provider Model Snapshot

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu -run '^TestDVFEI008' -count=1`
- **Compile Red result**: The desired test could not compile because Feishu had no task-scoped provider metadata snapshot boundary.
- **Behavior Red command**: The same focused command after adding snapshot and engine-only application.
- **Behavior Red result**: Engine received `claude-4-sonnet` while compactor model remained empty, reproducing the configuration divergence.
- **Green implementation**: Runner snapshots provider protocol and selected model once at task start, applies the same immutable value to `engine.Config` and `CompactionConfig`, and then constructs both collaborators. Explicit engine model metadata prevents a second dynamic provider read; compactor resolves known-model context windows from that same value.
- **Green commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu ./internal/engine ./internal/compaction -count=1` and `env GOCACHE=/tmp/fox-go-build-cache go test -race ./internal/feishu -run '^TestDVFEI008' -count=1 -v`.
- **Green result**: PASS; `DV-FEI-008` is corrected with one rotating-provider metadata read and matching engine/compactor configuration.

## T082 / D-FEI-009: Correlated Panic Outcome and Terminal Delivery

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu -run '^TestDVFEI009' -count=1`
- **Compile Red result**: The desired test could not compile because Feishu defined neither `TaskOutcome` nor an outcome observer boundary.
- **Behavior Red command**: The same focused command after adding only the outcome contract and Runner injection point.
- **Behavior Red result**: Existing recovery logged the panic but emitted zero task outcomes and made no terminal delivery attempt.
- **Green implementation**: Scheduler recovery now cancels the task context after stack cleanup, emits one failed `TaskOutcome` correlated by task/chat through a non-blocking observer contract, and attempts one bounded failure notification using a fresh background-derived context rather than the cancelled task context. Observer panic and transport failure cannot prevent scheduler completion; production Runner defaults to a logging task-outcome observer.
- **Green commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1`, `env GOCACHE=/tmp/fox-go-build-cache go test -race ./internal/feishu -run '^TestDVFEI009' -count=1 -v`, and `env GOCACHE=/tmp/fox-go-build-cache go vet ./internal/feishu ./cmd/feishu`.
- **Green result**: PASS; `DV-FEI-009` is corrected with exactly one outcome, one deadline-bearing successful terminal delivery fixture, and verified successor execution.

## T083 / D-FEI-010: Typed Observable Delivery Failures

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu -run '^TestDVFEI010' -count=1`
- **Compile Red result**: The desired tests could not compile because Feishu defined no delivery stage/failure/observer types, no stage-aware Runner methods, and no global transport text bound.
- **Behavior Red command**: The same focused command after adding only the contracts, injection points, send helpers, and pre-transport truncation call.
- **Behavior Red result**: All seven failed sends were only logged and yielded zero observer records; the nominal 1800-rune truncation produced 1824 runes after appending its marker.
- **Green implementation**: Added typed `DeliveryFailure` records for receipt, session, lifecycle, final, ordinary-failure, panic-failure, and cancellation stages. Runner and Reporter route every transport error through an injected non-blocking observer contract with panic isolation; queued/active cancellation and panic use fresh bounded delivery contexts. Messenger enforces a total 1800-rune pre-transport bound including its truncation marker. The Feishu composition root explicitly injects the production logging observer.
- **Green commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu ./internal/approval ./internal/engine ./internal/compaction -count=1` and `env GOCACHE=/tmp/fox-go-build-cache go test -race ./internal/feishu ./cmd/feishu -run 'TestDVFEI009|TestDVFEI010|TestDVFEI006' -count=1 -v`.
- **Green result**: PASS; `DV-FEI-010` is corrected, all seven stages preserve correlation and wrapped cause, text is bounded before the fake transport, production observation is explicit, and the T040 correction stop is cleared.

## T041: AgentOps Residual Defect Verification

- **Workflow**: Verification-only task. Tests preserve controlled current behavior; no AgentOps production correction semantics were inferred or implemented.
- **Commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./cmd/agentops ./internal/agentops -run 'TestDVAOP|TestDVAOPApproval' -count=1 -timeout=30s`, full AgentOps package tests, and the same verification set under `-race`.
- **Result**: PASS, with all six risks classified as proven defects against production source commit `3dd89e1`.
- **DV-AOP-001**: Shared Gateway validation now rejects missing/empty message IDs, but AgentOps adds a second process-local Deduper that accepts an empty key directly, claims before bridge delivery, never rolls back for bridge/task failure or completion, expires strictly after TTL, and forgets state on restart.
- **DV-AOP-002**: Background HTTP, bridge, two task channels, and Runner have no coordinated shutdown. Runner returns before active work and cancellation drops an already accepted permit waiter without execution or terminal correlation.
- **DV-AOP-003**: Panic recovery logs and releases capacity but emits no terminal delivery. Ordinary/timeout failures attempt one fallback, while parent cancellation uses an already-cancelled delivery context; no typed exactly-once terminal authority exists.
- **DV-AOP-004**: Selected provider model is not propagated into `CompactionConfig.Model`, so registered context-window selection falls back.
- **DV-AOP-005**: Initial delivery error is ignored, unbounded final content crosses the Runner messenger boundary, final failure triggers one fallback, and fallback failure is discarded with no controlling outcome.
- **DV-AOP-006**: Lexically valid service names can resolve through a symlink outside `logDir`; controlled tests also prove the independent 200-line and one-MiB-per-line bounds remain effective.
- **Shared approval reuse**: PASS. AgentOps constructs one `approval.Store`, shares it between Gateway and Runner, and the authenticated callback preserves corrected 204/409/404 exactly-once behavior. No second approval protocol is present or required.
- **Gate outcome**: AgentOps correction stop is active. Baseline fixture generation and production architecture migration remain blocked until the user separately confirms correction semantics and each defect follows Red-Green-Refactor in an independent Green commit.

## T084 / D-AOP-001: One Durable Task-Acceptance Authority

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./cmd/agentops -run '^TestDVAOP001' -count=1`
- **Compile Red result**: The corrected contract could not compile because AgentOps exposed no `newDeliveryStore` composition boundary and still owned the process-local `Deduper`.
- **Green implementation**: AgentOps now constructs the shared Feishu `FileDeliveryStore` at `~/.foxharness/feishu/deliveries.json`, installs it on the Gateway, and removes the complete process-local Deduper and its pre-bridge claim. The Gateway remains the sole acceptance authority and retains its invalid-ID rejection, concurrent reservation, durable duplicate acknowledgement, and live enqueue rollback semantics.
- **Green commands**: `env GOCACHE=/tmp/fox-go-build-cache go test ./cmd/agentops ./internal/agentops ./internal/feishu -count=1`, focused delivery tests, `env GOCACHE=/tmp/fox-go-build-cache go test -race ./cmd/agentops ./internal/feishu -run 'TestDVAOP001|TestFileDeliveryStoreConcurrentReservationHasOneWinner|TestGatewayRollsBackReservation' -count=1`, and the architecture test.
- **Green result**: PASS; `DV-AOP-001` is corrected. AgentOps and Feishu share one persisted message-ID namespace, sequential and restart duplicates enqueue once, concurrent reservation has one winner, and a live unavailable enqueue remains retryable after rollback.

## T085 / D-AOP-002: Coordinated Two-Channel Shutdown

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./cmd/agentops ./internal/agentops -run '^TestDVAOP002' -count=1 -timeout=10s`
- **Compile Red result**: The corrected composition contract could not compile because AgentOps had no lifecycle-owning `serve` function; the old Runner tests also described immediate return and dropped permit waiters.
- **Green implementation**: The composition root now owns one signal context and one shutdown deadline, stops and joins HTTP intake before closing its output, drains a dedicated Feishu-to-AgentOps bridge that exclusively closes the downstream channel, cancels task contexts, and waits for the bridge and Runner. `Runner.Start` consumes until producer closure and joins every worker, so cancellation cannot silently discard an accepted permit waiter.
- **Green commands**: focused `DV-AOP-002` tests, `env GOCACHE=/tmp/fox-go-build-cache go test ./cmd/agentops ./internal/agentops ./internal/feishu -count=1 -timeout=60s`, focused `-race` tests, and the architecture test.
- **Green result**: PASS; `DV-AOP-002` is corrected. Ordinary producer closure drains accepted work, signal cancellation preserves producer-safe channel ownership, and active or queued tasks are accounted for under the single process deadline.

## T086 / D-AOP-003: Exactly-One Typed Task Outcome

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/agentops -run '^TestDVAOP003' -count=1 -timeout=10s`
- **Compile Red result**: The corrected contract could not compile because AgentOps defined no task outcome status, reason, record, or observer.
- **Green implementation**: Runner execution now returns one typed outcome for every path; the only completion function observes that outcome and attempts one non-success terminal notification with a fresh ten-second context. Timeout and parent cancellation map to `cancelled` with distinct reasons; ordinary errors and panic map to `failed`; success maps to `completed`. Scheduled workers release their concurrency permit before entering the terminal transition.
- **Green commands**: focused `DV-AOP-003` tests, `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/agentops ./cmd/agentops -count=1 -timeout=60s`, focused `-race` tests, and package `go vet`.
- **Green result**: PASS; `DV-AOP-003` is corrected. Exactly one correlated outcome is emitted per accepted task, panic permits a successor before a deliberately blocked observer returns, and all non-success terminal deliveries receive a live deadline-bearing context.

## T087 / D-AOP-004: Immutable Per-Task Provider Snapshot

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/agentops -run '^TestDVAOP004' -count=1`
- **Compile Red result**: The corrected contract could not compile because AgentOps had no provider snapshot type or constructor; production also left compaction model configuration empty.
- **Green implementation**: Each production task reads provider protocol and model once into a provider wrapper that delegates generation while returning frozen metadata. The same wrapper configures and drives the main engine, compactor, automemory hooks, and subagent manager, so child execution inherits the parent task identity without another mutable metadata read.
- **Green commands**: focused `DV-AOP-004` and race tests, architecture tests, plus `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./internal/subagent ./internal/agentops ./internal/engine ./internal/compaction ./cmd/agentops -count=1 -timeout=60s`.
- **Green result**: PASS; `DV-AOP-004` is corrected. A rotating provider is read once, engine and compactor receive `claude-4-sonnet`, compaction selects its registered non-default window, and child metadata remains frozen. The temporary `HOME` isolates existing subagent session tests from the sandboxed user directory while retaining the existing module cache explicitly.

## T088 / D-AOP-005: Typed Bounded Delivery Boundary

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/agentops ./cmd/agentops -run '^TestDVAOP005' -count=1`
- **Compile Red result**: AgentOps defined no delivery stages, failure record, observer, bound, or observer composition API; the production observer assertion also failed.
- **Green implementation**: All session, final, ordinary-failure, panic-failure, timeout, and cancellation text now crosses one Runner helper that bounds text to 1,800 runes before transport and observes typed correlated failures. Observer panic is isolated. A failed final send still causes one failure-delivery attempt, but failure of that attempt is only observed and cannot recursively send. Production installs the logging observer explicitly.
- **Green commands**: focused `DV-AOP-005`, `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/agentops ./cmd/agentops ./internal/feishu -count=1 -timeout=60s`, focused `-race`, source-call audit, and package `go vet`.
- **Green result**: PASS; `DV-AOP-005` is corrected. Session, final, and terminal failures remain separately observable, every transport receives bounded text, reason-to-stage mapping is typed, and task outcome remains independent from delivery outcome.

## T089 / D-AOP-006: Rooted Regular-File Log Access

- **Behavior Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/agentops -run '^TestDVAOP006' -count=1 -timeout=20s`
- **Behavior Red result**: A lexically valid service still opened an outside-root symlink and returned external log content.
- **Green implementation**: Production no longer joins a path and calls unrestricted `os.Open`. It opens the configured directory as `os.Root`, opens only the validated relative `<service>.log` through that root, and checks the opened handle is a regular file. The test-only reader seam remains available without weakening production open behavior.
- **Green commands**: focused security, cancellation, ordering, and resource-bound tests; focused `-race`; AgentOps/Feishu entry and package tests; architecture tests; package `go vet`; and the complete repository gate `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./... -count=1 -timeout=180s` outside the network-listen sandbox.
- **Green result**: PASS; `DV-AOP-006` is corrected and the T041 correction stop is cleared. Outside symlinks, traversal, separators, and non-regular targets fail closed while valid matching and all prior resource limits remain stable. The complete repository test suite passes.

## T042: Benchmark Defect Verification Gate

- **Proof command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark ./cmd/bench -run '^TestDVBEN' -count=1 -timeout=30s`
- **Proof result**: PASS, proving all seven suspected behaviors against production source and hermetic temporary fixtures.
- **DV-BEN-001/002**: No whole-case lifetime exists, cancellation does not stop all validation work, and failed evaluation results do not control process status.
- **DV-BEN-003/007**: Fidelity is manual rather than resolved, while results omit repeat, run, definition, fixture, runtime-status, provider, and model provenance.
- **DV-BEN-004**: Fixture copy follows outside file symlinks, file validation permits traversal and outside symlinks, and setup failures do not clean the already-created workspace.
- **DV-BEN-005/006**: Invalid and vacuous domains are accepted; command output is unbounded and cancellation targets only the immediate shell without explicit process-tree cleanup.
- **Supporting gates**: Focused `-race` and architecture tests PASS. Full benchmark packages PASS with a temporary `HOME`; the ordinary sandbox run only failed because an existing session test writes the real user home.
- **Gate outcome**: Benchmark correction stop is active. No production fix or baseline fixture is authorized until the seven correction semantics are separately confirmed and assigned independent Red-Green-Refactor commits.

## T090 / D-BEN-001: One Bounded Case Lifetime

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark -run '^TestDVBEN001' -count=1 -timeout=10s`
- **Compile Red result**: The corrected test could not compile because `Case` had no timeout, Runner had no case-lifetime seam, and validation records had no typed terminal status.
- **Green implementation**: `timeout_seconds` defaults to 600 and Runner derives one context for fixture copy, harness construction, engine execution, and validation. Validation records now distinguish passed, failed, cancelled, and timed out; cancellation or timeout synthesizes ordered records for every remaining entry without executing it. Command validation's existing two-minute child context naturally uses the earlier parent deadline.
- **Cross-validation correction**: An expired case context cannot govern terminal cleanup. DEC-044 and T093 now require failed-workspace cleanup under one fresh background-derived 30-second context, matching the bounded terminal cleanup principle found in both references.
- **Green commands**: focused Red/Green, full benchmark and command packages with hermetic `HOME`, focused `-race`, and package `go vet`.
- **Green result**: PASS; `DV-BEN-001` is corrected without implementing later exit, fidelity, path, input-domain, process-tree, or provenance corrections early.
