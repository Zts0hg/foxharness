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

## T091 / D-BEN-002: Typed Repeat Status and Process Exit

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark ./cmd/bench -run '^TestDVBEN002' -count=1`
- **Compile Red result**: Result exposed no typed status or separated error evidence and `cmd/bench` exposed no result-derived exit decision.
- **Green implementation**: Runner initializes a partial result before setup and classifies completion, runtime/evaluation failure, parent cancellation, case timeout, and infrastructure failure without conflating their evidence. The command appends any partial result before handling its infrastructure error, writes available summary and JSON, and returns 0/1/2 from aggregate semantics rather than `log.Fatal` side effects.
- **Green commands**: focused Red/Green, full benchmark and command packages with hermetic `HOME`, focused `-race`, and package `go vet`.
- **Green result**: PASS; `DV-BEN-002` is corrected. Existing `Error` remains populated for current readers while typed status and three evidence fields become authoritative for the corrected pre-baseline schema.

## T092 / D-BEN-003: Resolved Benchmark Runtime Fidelity

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark ./cmd/bench -run '^TestDVBEN003' -count=1`
- **Compile Red result**: No typed runtime specification constructor existed and production still contained manually maintained fidelity literals.
- **Green implementation**: `BenchmarkRuntimeSpec` freezes provider/model, turn budget, ordered tool surface, and prompt, memory, compaction, permission, observation, and interaction policies. Composition uses its values for engine and compactor construction and calls `Fidelity()` for both machine-readable snapshot and derived human claims.
- **Green commands**: focused Red/Green, full benchmark and command packages with hermetic `HOME`, focused `-race`, and package `go vet`.
- **Green result**: PASS; `DV-BEN-003` is corrected without introducing the future general runtime profile implementation.

## T093 / D-BEN-004: Rooted Fixture and Workspace Lifecycle

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark -run '^TestDVBEN004' -count=1 -timeout=30s`
- **Compile Red result**: Corrected lifecycle tests could not compile because Runner exposed neither a context-aware workspace-removal seam nor typed cleanup evidence. The existing path test also demonstrated outside-root fixture symlink copying and `file_contains` traversal/symlink acceptance.
- **Green implementation**: Fixture reads and destination creation now use separate `os.Root` authorities, accept directories and regular files only, and reject symlinks and unsupported entries without mutating the source. `file_contains` validates every rooted component, rejects traversal, symlinks, and non-regular targets, then reads the opened regular handle. Successful workspaces remain retained; all non-success paths remove the workspace under a fresh background-derived context with a 30-second default, and cleanup failure or timeout is returned and recorded as infrastructure evidence.
- **Green commands**: focused `DV-BEN-004` plus success-retention coverage, full benchmark and command packages with hermetic `HOME`, focused `-race`, architecture tests, and package `go vet`.
- **Green result**: PASS; `DV-BEN-004` is corrected. Partial-copy, factory, cancellation, cleanup failure, and cleanup timeout paths are explicit, while successful workspace inspection remains unchanged.

## T094 / D-BEN-005: Strict Benchmark Input Domains

- **Behavior Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark ./cmd/bench -run '^TestDVBEN005' -count=1 -timeout=30s`
- **Behavior Red result**: Production accepted unknown YAML fields, whitespace-only required values, invalid turn and timeout domains, unknown or vacuous validations, and irrelevant validation fields; relative fixtures remained unresolved, while zero and negative repeat counts returned success after writing an empty report. A test-only missing import was corrected and Red was rerun so every remaining failure came from production behavior. A second Red proved that explicit YAML nulls still bypassed pointer/zero-value validation.
- **Green implementation**: Case loading uses strict known-field decoding plus mapping-shape metadata so omitted defaults remain distinct from explicit null or irrelevant field presence. It validates top-level fields, validation count, numeric domains, and each ordered validation before resolving a relative fixture against the case-file directory. The CLI rejects non-positive repeats before case loading or reporting; the standard flag parser rejects integer overflow. The checked-in counter-race fixture reference now follows the corrected relative-path contract.
- **Green commands**: focused `DV-BEN-005`, full benchmark and command packages with hermetic `HOME`, focused `-race`, architecture tests, and package `go vet`.
- **Green result**: PASS; `DV-BEN-005` is corrected. Invalid structure cannot produce an empty successful run or reach fixture/runtime work, and valid omitted defaults remain 12 turns and 600 seconds.

## T095 / D-BEN-006: Bounded Validator Process Trees

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark -run '^TestDVBEN006' -count=1 -timeout=20s`
- **Compile Red result**: Corrected tests could not compile because benchmark exposed no dedicated command executor, one-MiB stream limit, independent output/overflow evidence, or configurable termination protocol. The prior proof had already established unbounded `CombinedOutput` and immediate-shell-only cancellation.
- **Green implementation**: Command validation now owns a dedicated executor with independent synchronized stdout/stderr buffers capped at one MiB each. Every process enters a platform-specific tree boundary; overflow or context termination sends TERM, waits a bounded grace period, escalates to KILL, and waits for reaping. Results retain separate bounded streams and typed overflow flags while cancellation and timeout remain distinct. Windows uses tree-aware `taskkill`, with a conservative fallback for other platforms.
- **Green commands**: focused `DV-BEN-006`, full benchmark and command packages with hermetic `HOME`, focused `-race`, architecture tests, package `go vet`, and `GOOS=windows GOARCH=amd64 go test -c ./internal/benchmark`.
- **Green result**: PASS; `DV-BEN-006` is corrected. Ignored-TERM descendants cannot leave a delayed side effect, active cancellation synthesizes ordered unstarted records, overflow remains a failed validation, and ordinary non-zero exits preserve both bounded streams.

## T096 / D-BEN-007: Versioned Reproducible Result Provenance

- **Compile Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark ./cmd/bench -run '^TestDVBEN007' -count=1 -timeout=30s`
- **Compile Red result**: Corrected tests could not compile because Result exposed no schema version, repeat/run/case/fixture identity, runtime terminal provenance, deadlines, or one-based repeat orchestration. A later behavior Red proved that writer accepted nil and unversioned results, and another proved wrapped runtime cancellation/deadline causes were classified as ordinary failure.
- **Green implementation**: `RunRepeat` snapshots case input, assigns one-based repeat identity, materializes the fixture, hashes a rooted deterministic manifest of the actual pre-runtime workspace, hashes the normalized semantic case definition, and captures the real Agent run ID through a no-op lifecycle reporter even on runtime failure. Result schema v1 separates aggregate and runtime status/cause, records provider/model, actual case and command deadlines, and retains the resolved runtime-fidelity snapshot. The writer rejects nil or non-v1 results, and CLI orchestration passes explicit one-based indices without untrusted capacity preallocation.
- **Compatibility fixture**: `internal/benchmark/testdata/benchmark-result-v1.golden.json` freezes deterministic success and runtime-failure JSON. Tests normalize only duration, workspace, session, run, and timestamps; case/fixture identities and all stable provenance remain exact.
- **Green commands**: focused TDD cycles, full benchmark and command packages, focused `-race`, architecture tests, package `go vet`, Windows cross-compilation, and `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./... -count=1 -timeout=180s` outside the network-listen sandbox.
- **Green result**: PASS; `DV-BEN-007` is corrected and the T042 correction stop is cleared. All seven benchmark defects now have independent Green commits, and the complete repository suite passes.

## T043: ChildRun Defect Verification Gate

- **Workflow**: Verification-only task. Hermetic tests distinguish the `delegate_task`/fork invocation adapters from `subagent.Manager` runtime lifecycle behavior; no production correction semantics were inferred or implemented.
- **Proof command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/subagent ./internal/app -run '^TestDVCHD' -count=1 -timeout=30s`.
- **Proof result**: PASS, proving all six suspected behaviors against production source commit `03f26ae` with temporary workspaces, isolated session homes, scripted providers, bounded local shell processes, and no external dependency.
- **DV-CHD-001**: A low-risk read-only delegation still advertises and executes arbitrary Bash. Direct shell redirection and a subprocess mutate both the workspace and an outside-root temporary file; excluding write/edit tools therefore does not enforce read-only effects.
- **DV-CHD-002**: The child model call records the selected 200K model, while the child compactor receives an empty model and resolves 128K. A deterministic 120K-token conversation triggers fallback compaction but remains below the selected-model trigger.
- **DV-CHD-003**: Model-visible definitions and runtime calls preserve the current one-level child ceiling and omit TODO/skill/delegation tools, but the read-only prompt still instructs write_file, edit_file, read_todo, and update_todo use. Prompt and tool/permission snapshots have different authorities.
- **DV-CHD-004**: Context cancellation kills the active Unix process group and cancellation cleans an in-flight approval without execution or grant. The broader lifecycle remains defective: Bash can detach a delayed process before returning, then ChildRun turn exhaustion returns while that process performs a late workspace mutation.
- **DV-CHD-005**: The slash executor supplies `agent`, and the app fork adapter preserves processed task, parent session, project instructions, provider, tool ceiling, and report, but explicitly drops the selected agent before `subagent.Request`.
- **DV-CHD-006**: Provider, tool-followed-by-provider, persistence, compactor construction, turn-limit, and cancellation failures all expose nil to the parent even when a correlated child session and partial model content exist. Engine partial-result semantics are erased by the wrapper.
- **Reference cross-validation**: Codex applies typed child role/model/depth to child config and source, and retains a runtime process registry with terminate-all cleanup. Claude Code resolves `subagent_type` into a concrete agent definition/model/tool set, performs command-level read-only Bash checks in restricted forks, filters recursive Agent capability, cleans agent shell tasks, and preserves max-turn reason plus latest text-bearing assistant content. These references support one frozen child snapshot and one lifecycle owner without requiring their background/team features.
- **Supporting gates**: Focused `-race`, related subagent/app/slash/tools/engine/compaction packages, `internal/architecturetest`, package `go vet`, and the complete repository test suite PASS with fixed `HOME`, `GOMODCACHE`, and `GOCACHE` where required. The full-suite sandbox run reached only the existing provider `httptest` listener restriction; the identical command passed outside that network-listen sandbox.
- **Gate outcome**: ChildRun correction stop is active. T044, baseline fixture generation, characterization freeze, and production architecture migration remain blocked until the user separately confirms correction semantics and each defect receives an independent Red-Green-Refactor correction commit.

## T097 / D-CHD-002: Frozen Child Execution Snapshot

- **Behavior Red command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/subagent -run '^TestDVCHD002' -count=1 -timeout=30s`.
- **Behavior Red result**: After manager construction, changing provider metadata changed the child engine's recorded model while compaction still used an empty-model 128K fallback. The invocation model identity, context window, trigger threshold, and resulting history projection therefore came from different snapshots.
- **Green implementation**: `subagent.Manager` now freezes provider protocol, model identity, and the registry-resolved context window at construction. Engine configuration and compactor construction consume that private snapshot. Known models retain their configured window; unknown models retain their observable identity and use one explicit default context window.
- **Green commands**: focused `DV-CHD-002`, relevant subagent/app/Feishu/AgentOps/engine/compaction package tests, focused `-race`, package `go vet`, and `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./... -count=1 -timeout=180s` outside the network-listen sandbox.
- **Green result**: PASS; `DV-CHD-002` is corrected. Provider metadata mutation cannot split child invocation telemetry from compaction behavior, and no target runtime-profile abstraction was introduced before the architecture migration phase.

## T098 / D-CHD-005: Resolved Child Agent Identity

- **Behavior Red command**: `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./internal/app -run '^TestDVCHD005' -count=1 -timeout=30s`.
- **Behavior Red result**: The fork adapter discarded explicit `general-purpose`, so the child prompt lacked its agent role; an unknown non-empty selector silently ran the current child and reached the provider instead of failing.
- **Propagation Red**: Focused subagent/app compilation failed because child `Result` and persisted `Session` exposed no resolved agent or parent-lineage fields. After those became Green, a separate execution Red proved that a disjoint caller/agent tool intersection became an empty slice that the registry misread as unrestricted and re-exposed all four tools. One initial compile attempt also lacked the test-only `reflect` import; it was corrected and rerun so the valid Red contained only missing production symbols.
- **Green implementation**: `AgentID` resolution occurs before session creation; empty and `general-purpose` select the built-in persona and all other non-empty values fail explicitly. The normalized identity enters request handling, role prompt, additive backward-compatible session lineage, typed result, and run trace. Agent tool policy is structurally limited to persona plus tool narrowing and uses a stable intersection where nil means unrestricted and an explicit empty slice means no tools; it cannot alter provider, model, workspace, depth, permission, or turn ceilings.
- **Green commands**: focused `DV-CHD-005` and tool-ceiling tests, subagent/app/session/engine/slash/tools package tests, focused `-race`, package `go vet`, the complete `DV-CHD` gate, and `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./... -count=1 -timeout=180s` outside the network-listen sandbox.
- **Green result**: PASS; `DV-CHD-005` is corrected without adding another agent persona, model selection, deeper child nesting, or target runtime-profile code. T099 retains responsibility for deriving every tool, permission, alias, parallel-safety, and prompt consumer from one immutable effective snapshot.

## T099 / D-CHD-003: Immutable Effective Child Tool Snapshot

- **Behavior Red command**: `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./internal/subagent -run '^TestDVCHD003' -count=1 -timeout=30s`.
- **Behavior Red result**: Read-only, writable, and single-tool child runs exposed the correct definitions and execution surface but inherited hard-coded write, TODO, skill, and interaction claims. An explicit empty caller ceiling also collapsed to nil and restored all tools. A fixture-only nil-versus-empty expectation was corrected and rerun after production behavior became Green.
- **Additional Reds**: Canonical allowlists dropped the selected tool's alias from definitions, execution, permission metadata, and parallel-safety; a consumer could mutate the snapshot's later-turn input schema through a shared nested map; and capability-scoped Composer instances still rendered optional interaction and skill guidance. The schema-mutation test initially assumed a concrete map instead of `interface{}` and was corrected before the valid production Red.
- **Green implementation**: Child assembly builds one privately owned registry from the access-mode ceiling, agent/caller intersection, canonical alias expansion, and permission decoration, then freezes model-visible definitions with recursive JSON-like schema copies. Execution, aliases, permission assessment, parallel-safety, and turn boundaries delegate through that same snapshot. Composer receives only the frozen model-visible names and conditionally renders base, TODO, persistent-memory, interaction, and skill fragments; unscoped CLI/TUI/remote composition retains its prior full prompt path.
- **Green commands**: focused behavior, alias, schema-immutability, and optional-guidance tests; complete context/subagent/tools/permission/app/engine/slash packages; focused `-race`; package `go vet`; the complete `DV-CHD` gate; and the complete repository suite outside the network-listen sandbox.
- **Green result**: PASS; `DV-CHD-003` is corrected. One first full-suite attempt hit the existing 50ms Bash timeout/truncation fixture before child output appeared; that focused test passed ten consecutive runs and the unchanged full-suite command then passed completely. No TODO, model-invocable skill, interaction, or nested-delegation capability was added to ChildRun.

## T100 / D-CHD-001: Fail-Closed Read-Only Child Bash

- **Behavior Red command**: `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build go test ./internal/tools ./internal/toolpolicy ./internal/permission ./internal/context ./internal/subagent -count=1 -timeout=120s`.
- **Behavior Red result**: The corrected ChildRun test first proved that a read-only registry and even a FullAccess coordinator executed direct and subprocess mutations. Successive Reds showed that background and compound syntax, empty input, unrestricted runner fallback, inherited environment, over-broad prompt guidance, and a sandbox-free execution seam were still accepted or unrepresented. Darwin execution Reds then established the required deny-by-default profile and exposed the exact traversal and system-service reads needed to start a contained shell.
- **Green implementation**: Read-only ChildRun now installs a Bash-compatible tool with the unchanged name and input schema but an explicit capability description. A complete shell AST pass accepts only a conservative read-only command and flag allowlist and rejects redirects, substitutions, assignments, compound or dynamic execution, and background or coprocess forms. Accepted commands can reach only a platform runner with a fixed environment and declared readable roots. Darwin uses `sandbox-exec` with default, network, and file-write denial plus precise read roots; sandbox setup failure maps to one unavailable sentinel. Other platforms currently fail closed. Permission approval and FullAccess cannot expand the read-only ceiling, and timeout guidance no longer suggests forbidden background execution.
- **Containment verification**: The host Darwin test established the real sandbox, read a workspace fixture, rejected workspace writes and outside-root reads, and did not inherit parent secrets or proxy variables. Direct runner and end-to-end tests also prove that unavailable containment never invokes ordinary `RunBashCommand`.
- **Compatibility adjustment**: Existing `DV-CHD-004` lifecycle and child-evidence fixtures now request writable ChildRun Bash because their subjects are process cleanup and evidence aggregation, not the corrected read-only capability. Their prior assertions remain unchanged and Green.
- **Green commands**: related tools/toolpolicy/permission/context/subagent/app/engine/slash packages; focused `-race`; package `go vet`; Linux and Windows tools test-binary cross-compilation; and `env HOME=/tmp/fox-test-home XDG_CONFIG_HOME=/tmp/fox-test-home/.config XDG_DATA_HOME=/tmp/fox-test-home/.local/share GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build go test ./... -count=1 -timeout=180s` outside the network-listen and nested-sandbox restrictions.
- **Green result**: PASS; `DV-CHD-001` is corrected. Model-visible presence and schema remain compatible, safe workspace inspection works on Darwin, unsupported platforms reject explicitly, and no parser, permission, or platform-unavailable path can execute unrestricted Bash.

## T101 / D-CHD-006: Typed Correlated Child Outcomes

- **Compile Red command**: `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build go test ./internal/subagent -run '^TestDVCHD006' -count=1 -timeout=30s`.
- **Compile Red result**: The corrected matrix could not compile because ChildRun exposed no outcome status, invocation or run correlation, typed outcome error, or session-creation seam. The existing manager returned nil for provider, persistence, compactor-construction, turn-exhaustion, cancellation, rejection, and start failures.
- **Behavior Reds**: After the manager outcome became Green, delegate and app fork adapters still discarded the outcome whenever error was non-nil; slash executor and model-invoked fork skill discarded the simultaneous partial text; the generic tool registry retained only the terminal error; and TUI rendered only the error while dropping the fork result. A fixture-only missing `time` import and one mechanically misplaced app error block were corrected before rerunning their production Reds.
- **Green implementation**: `subagent.Result` now represents one terminal outcome with a fresh invocation ID, parent and agent lineage, typed status, and session/run IDs as soon as each exists. A private engine reporter records run identity and only `OnMessage` callbacks issued after successful authoritative assistant-message persistence; streaming deltas, pure tool calls, tool output, and failed appends cannot become Report. `TurnLimitError` adds typed exhaustion while preserving the exact existing error text. Cancellation remains errors.Is-compatible, and rejection or pre-run failures retain their established identities without inventing session/run IDs.
- **Adapter propagation**: `OutcomeError` carries the typed result and unwraps the original cause. Delegate and fork adapters retain both values; their structured tool paths mark failure and label committed text `Partial Report`. Slash executor, fork skill, and TUI preserve the same evidence, with TUI using an error entry rather than an assistant success entry. Successful delegate and fork output formats remain unchanged.
- **Compatibility adjustment**: The existing `DV-CHD-005` unknown-agent and `DV-CHD-004` turn-exhaustion fixtures now assert the corrected typed rejection/exhaustion while retaining their original no-session and late-side-effect checks.
- **Green commands**: complete affected subagent/app/slash/skilltool/tools/engine/session/TUI packages; complete `DV-CHD` gate; focused `-race`; package `go vet`; and `env HOME=/tmp/fox-test-home XDG_CONFIG_HOME=/tmp/fox-test-home/.config XDG_DATA_HOME=/tmp/fox-test-home/.local/share GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build go test ./... -count=1 -timeout=180s` outside the network-listen sandbox.
- **Green result**: PASS; `DV-CHD-006` is corrected. All required terminal classes produce one correlated outcome, original errors retain classification, partial text is authoritative and explicitly non-successful, and every affected adapter preserves it through the user/model-visible boundary.

## T102 / D-CHD-004: Run-Owned Synchronous Shell Supervision

- **Compile Red command**: `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build go test ./internal/tools ./internal/subagent -run '^(TestSupervisedBash|TestBashProcessSupervisor|TestDVCHD004)' -count=1 -timeout=45s`.
- **Compile Red result**: Corrected tests could not compile because tools exposed neither a supervised Bash constructor nor a process supervisor, and Manager had no run-owned supervisor factory. The existing `DV-CHD-004` proof still observed a delayed post-exhaustion mutation.
- **Behavior Reds**: Initial Green work showed Bash-dialect `coproc` was not classified by the default parser and that assigning `exec.Cmd.Cancel` to a command created without `CommandContext` prevented startup. After those were fixed, a panic Red proved the Manager cleanup defer retained session but not the run ID already emitted by the engine reporter. Each issue was corrected and rerun before broadening the gate.
- **Green implementation**: A complete Bash-dialect synchronous classifier rejects background statements, coprocesses, process substitution, detach wrappers, dynamic command wrappers, and nested interpreters before the ChildRun runner. Writable ChildRun alone receives `NewSupervisedBashTool`; top-level and direct ordinary Bash behavior remains unchanged. `BashProcessSupervisor` registers commands before Start, creates platform process-tree boundaries, bounds output, waits through TERM-to-KILL escalation, kills residual groups after shell exit, removes only completed entries, stops admission during Cleanup, and reports reap failure or timeout explicitly.
- **Lifecycle ownership**: Each `Manager.Run` constructs exactly one supervisor and a cancellable run context. One deferred terminal block catches panic, retains reporter-established run/partial identity, cancels pending permission work, performs cleanup under a fresh five-second context, and joins cleanup evidence with the original error before returning. Cleanup failure changes even a successful engine result to `OutcomeFailed`; no terminal outcome is visible until cleanup completes.
- **Cross-platform hardening**: Unix uses a dedicated process group and TERM/KILL by negative PGID. Windows uses tree-aware `taskkill` and treats an already completed process as clean; other platforms use interrupt/kill. The final-timeout path avoids reading wait state until channel synchronization, preventing a race with a still-active wait goroutine.
- **Green commands**: complete affected tools/toolpolicy/permission/subagent/app/slash/engine/TUI packages; complete `DV-CHD` and architecture gates; focused `-race`; package `go vet`; Linux tools and Windows tools/subagent cross-compilation; and the complete repository suite outside the network-listen sandbox.
- **Green result**: PASS; `DV-CHD-004` is corrected and the T043 correction stop is cleared. All six ChildRun defects now have independent Green corrections, no delayed shell side effect survives exhaustion, pending approval cancellation remains clean, and T044 may proceed.

## T044: Autodev Defect Verification Gate

- **Workflow**: Verification-only task. One hermetic test file exercises ledger durability, resume-stage validity, backlog identity and content authority, artifact materialization and containment, command resources and process trees, GitHub issue correlation, asynchronous extraction ownership, concurrency configuration, and partial core outcomes. No production behavior was changed.
- **Proof command**: `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/autodev -run '^TestDVAUT' -count=10 -timeout=90s`.
- **Proof result**: PASS on ten consecutive runs, proving all ten suspected behaviors against production source commit `c8bb3cf` with temporary repositories/filesystems, scripted Git/GitHub/core boundaries, a self-hosted helper process, fixed clocks, and no network, credentials, ambient HOME, or uncontrolled sleeps.
- **DV-AUT-001**: Initial ledger-save failure stops before worktree creation. Every later transition-save failure only warns, while in-memory execution continues through remaining SDD work, issue, PR, done reporting, and worktree cleanup, leaving remote side effects without recoverable durable state.
- **DV-AUT-002**: Empty and known SDD stages map predictably, but every non-empty unrecognized value is treated like a post-pipeline state. A malformed future stage therefore bypasses all SDD stages, publishes, and becomes done.
- **DV-AUT-003/004**: Duplicate-title queue consumption and matching-description refresh are safe subcases. The broader identity/content boundary remains defective: rename duplicates identity, deletion retains stale pending work, reorder keeps old ledger order, removed records lose descriptions, multiline Markdown/code is flattened, text truncates at 4,000 runes, and a valid line over one MiB fails parsing.
- **DV-AUT-005/006**: Absolute FeatureDir is lexically reanchored, but traversal and directory/final-file symlinks escape materialization and verification. Command output is unbounded, cancellation leaves a descendant alive, and later gates still start under an already-cancelled context.
- **DV-AUT-007/008**: Issue verification is title-only, first-match, closed-inclusive, and capped at 20; a recorded number bypasses restart verification without stable correlation. Extraction remains non-terminal and panic-isolated, but Autodev exposes no core-runtime drain and removes the worktree while post-run work can remain pending.
- **DV-AUT-009/010**: Arbitrary concurrency values are accepted silently while execution always remains serial. Every partial result+error category loses session/run correlation and durable partial text before verification or Engineer handling.
- **Reference alignment**: The already confirmed local Codex and Claude Code comparison supports durable task identity and explicit terminal state, fail-closed unknown state, complete task text, rooted/canonical path authorities, bounded process trees and outputs, linked cancellation and cleanup, durable remote correlation, runtime-owned asynchronous lifecycle, validated configuration domains, and typed partial outcomes. It does not justify importing cloud-task, background-worker, resumable-message, or multi-worker features into Fox.
- **Supporting gates**: Focused `-race`, full `internal/autodev`, fixed-HOME `internal/app` and `cmd/fox`, and package `go vet` PASS. The first app package run used ambient HOME and failed only at the sandbox's real-home write restriction; the identical fixed-HOME command passed.
- **Gate outcome**: All ten rows are `blocked-defect`. The T044 correction stop is active: T045, characterization freeze, and production architecture migration remain blocked until the user separately confirms correction semantics and each defect is corrected through its own Red-Green-Refactor commit.

## T104 / D-AUT-002: Versioned Typed Pipeline Recovery

- **Compile Red command**: `env GOCACHE=/private/tmp/foxharness-go-build go test ./internal/autodev -run '^(TestDVAUT002|TestLoadLedgerMigratesKnownLegacyStage)' -count=1 -timeout=90s`.
- **Compile Red result**: Corrected tests could not compile because the ledger exposed neither a typed stage vocabulary, explicit stage transition state, nor `InvalidLedgerStateError`. The retained T044 proof showed that every unknown non-empty stage skipped the SDD pipeline and proceeded to publication.
- **Green implementation**: Ledger schema v1 persists one closed `PipelineStage` and `pending`, `running`, or `verified` `StageState`. Load validates current records before preconditions or any external work, rejects unsupported versions, illegal combinations, verified issue/PR stages without positive bindings, and versionless records that unexpectedly contain a transition state, and explicitly migrates only known legacy states. Candidate writes are also validated before persistence. Every SDD and publication stage records running intent before execution and verified state afterwards; issue/PR binding and verification share one authoritative commit.
- **Recovery semantics**: A verified stage advances to its successor. A running SDD or remote stage uses `ResumeStep` to evaluate ground truth before core execution, persists verified when already satisfied, and reruns only when verification fails. Remote resume order remains monotonic across enabled profile steps, and malformed state cannot be interpreted as a post-pipeline position.
- **Durability coverage**: The T103 failure matrix now injects failure at all twenty-three seed-following commits, including every new SDD and remote verified transition. Each failure stops dependent work, suppresses excess reports, and retains the worktree.
- **Green commands**: complete `internal/autodev`; focused `DV-AUT-002`; `go test -race ./internal/autodev -count=1 -timeout=240s`; `go vet ./...`; and `go test ./... -count=1 -timeout=300s` outside the restricted filesystem/listener sandbox.
- **Green result**: PASS; `DV-AUT-002` is corrected. The first complete-suite run failed only because the sandbox denied ambient `~/.foxharness` writes and local `httptest` listeners; the identical outside-sandbox run passed all packages. T105 is now unblocked while the broader T044 correction stop remains active.

## T105 / D-AUT-003: Immutable Item Identity and Requirement Reconciliation

- **Compile Red command**: `env GOCACHE=/private/tmp/foxharness-go-build go test ./internal/autodev -run '^(TestDVAUT003|TestParseOptionalSourceID)' -count=1 -timeout=90s`.
- **Compile Red result**: Corrected tests could not compile because source items and ledger records exposed no `SourceID`, immutable `ItemID`, reconciliation error, source lifecycle state, or persisted frozen revision. The retained T044 proof showed rename duplication, stale runnable entries, old scheduling order, and requirement loss.
- **Successive Reds**: After the initial identity model became Green, a unique v1 in-progress record still rejected its only safe one-time description binding, generic workflow `Commit` could rewrite `ItemID` or a complete frozen title/description/hash tuple and persist it, and the in-progress/no-stage validation branch skipped the v2 frozen-revision invariant. Focused tests proved all three defects before their corrections.
- **Green implementation**: Backlog parsing accepts optional normalized `**ID**`. Schema v2 persists immutable item/source identity, complete title and description, description byte length and SHA-256, revision freeze, source order, and `current`, `orphaned`, `blocked`, or `historical` state. Explicit source identity permits safe rename and reorder. No-ID records match only one exact title; duplicate source IDs, duplicate no-ID titles, rename/replacement ambiguity, and multiple legacy matches return actionable `ReconciliationError` without guessing.
- **Legacy and active state**: v0/v1 records receive deterministic legacy item identity and a durable `legacy_binding_pending` marker. A unique title may bind once and recover the full current revision, including after a missing active record was saved and reloaded as blocked. Missing pending items become non-runnable orphaned, missing in-progress items become blocked errors, and missing done items remain historical. Pending source fields and order may refresh; transition to in-progress freezes the revision, and later source edits block without mutating it.
- **Mutation boundary**: `Seed` is the only source reconciliation path. `Mark` and transactional `Commit` reject changes to item/source identity, slug, title, description, requirement metadata, priority, source state/order, and the freeze contract before in-memory or durable authority changes. Orchestration saves blocked/orphaned state before returning reconciliation failure and starts no worktree, core, issue, or PR work.
- **Green commands**: focused `DV-AUT-002/003` and parser migration tests; complete `internal/autodev`; `go test -race ./internal/autodev -count=1 -timeout=240s`; `go vet ./...`; and `go test ./... -count=1 -timeout=300s` outside the restricted filesystem/listener sandbox.
- **Green result**: PASS; `DV-AUT-003` is corrected and T104 stage-state guarantees remain Green under schema v2. T106 is now unblocked while the broader T044 correction stop remains active.

## T106 / D-AUT-004: Exact Bounded Requirement Authority

- **Red evidence**: Corrected tests proved Scanner rejected a valid line over one MiB, multiline and fenced Markdown collapsed, authoritative text truncated at 4,000 runes, fenced metadata and headings became control syntax, and a non-empty stale truncated artifact skipped rematerialization.
- **Green implementation**: `Parse` performs a bounded 64-MiB whole-file read, validates UTF-8 before parsing, normalizes CRLF/CR to LF, and preserves description internals including fenced control-shaped text. Oversize and invalid input fail atomically. Ledger revision identity uses the effective title fallback when description is empty.
- **Materialization**: The generated document records item identity, UTF-8 byte length, SHA-256, and a verbatim authoritative section. Single-line bounded text remains presentation-only. Existing requirements are reused only when all identity markers and exact authority match; stale pre-correction files are rebuilt.
- **Green gates**: focused and complete `internal/autodev`, focused race, `go vet ./...`, and `go test ./... -count=1 -timeout=300s` outside the restricted filesystem/listener sandbox all PASS. `DV-AUT-004` is corrected and T107 is unblocked.

## T107 / D-AUT-005: Rooted Feature Workspace Authority

- **Behavior Red evidence**: Corrected tests showed that materialization and verification accepted absolute, traversal, malformed, directory-symlink, final-file-symlink, and non-regular targets. A versionless ledger retained an escaping feature directory, and a provisioned directory symlink was rejected only after a core runner had already been constructed.
- **Green implementation**: `featureWorkspace` is the sole CodexSpec artifact authority. It validates the exact normalized `.codexspec/specs/<feature-name>` logical form, anchors worktree and feature handles with `os.Root`, creates directories one component at a time, and uses no-follow metadata before and after rooted opens. Reads and verification require regular final files. Materialization rejects unsafe existing targets and writes through a synced rooted temporary regular file plus rename rather than following a final target.
- **Recovery and execution order**: Ledger validation rejects illegal feature bindings for current and migrated legacy records before preconditions or persistence. Resume preflight checks every authoritative artifact target. Core construction is lazy, so deterministic materialization and workspace validation complete before any core runner exists; violations retain the worktree and cannot be silently reanchored or rewritten.
- **Fixture correction**: The E2E Git fake previously treated the branch argument in `git worktree add -b` as the worktree path. Legacy `MkdirAll` behavior hid that error by recreating the missing path. The fake now creates the actual declared worktree, and the resume fixture creates its persisted feature workspace explicitly.
- **Green gates**: focused `DV-AUT-005`, complete `internal/autodev`, `go test -race ./internal/autodev -count=1`, `go vet ./...`, and `go test ./... -count=1` outside the restricted build-cache/listener sandbox all PASS. `DV-AUT-005` is corrected and T108 is unblocked.

## T108 / D-AUT-006: Bounded Supervised Command Execution

- **Interface Red**: Corrected tests could not compile because Git and arbitrary command runners returned one unbounded combined string and exposed neither independent streams nor typed overflow. The retained proof showed a two-MiB stream was kept completely, cancellation left a descendant alive, and all later gate commands started with an already-canceled context.
- **Successive Reds**: After the structured runner became Green, a core attempt still had no default deadline and canceled `RemotePublisher` persisted the next remote intent before `StageMachine` observed cancellation. Dedicated tests proved both windows before production correction.
- **Structured result and consumers**: `CommandResult` retains stdout and stderr independently up to one MiB each, per-stream overflow evidence, and exit code. Every machine-readable Git and GitHub consumer parses stdout only and joins any overflow as `CommandOverflowError`; valid truncated JSON is never accepted. Quality gates keep ordinary command exit semantics, retain both diagnostic streams, and mark `OutputTruncated` with an explicit message.
- **Deadlines and supervision**: Read-only Git/GitHub queries default to 30 seconds, quality-gate commands to 10 minutes, and mutating worktree plus stage/core/Engineer attempts to 30 minutes; an earlier caller deadline wins. Commands enter a process-group or platform-equivalent boundary before start. Cancellation sends TERM, waits 250 milliseconds, escalates to KILL, and waits under a fresh bounded cleanup context. Final group cleanup also runs when the direct parent exits first, and cleanup failures remain in the returned error.
- **Cancellation and CLI**: Gate, stage, orchestrator, and remote loops check cancellation before starting or durably recording later work. `fox autodev` registers SIGINT and SIGTERM, cancels the same run context, preserves cancellation compatibility, and maps terminal status to 130 and 143 respectively; other launch modes retain their prior entry behavior.
- **Green gates**: complete `internal/autodev` and `cmd/fox`; focused `DV-AUT-006`; combined race; `go vet ./...`; Windows Autodev/CLI and Linux Autodev test-binary cross-compilation; and `go test ./... -count=1` outside the restricted build-cache/listener sandbox all PASS. `DV-AUT-006` is corrected and T109 is unblocked.

## T109 / D-AUT-007: Durable Issue Correlation and Logical Event Outbox

- **Compile Red**: Corrected marker cases could not compile because issue verification exposed no immutable item marker, typed identity conflict, error-capable preflight, durable remote event, or ledger outbox. The retained proof bound mutable duplicate titles from at most twenty results and trusted a recorded number without reading it.
- **Successive Reds**: Atomic publication tests proved binding lacked a pending event and restart could not distinguish delivery state. Ledger copy tests then proved a returned outbox slice could mutate authority. Adapter tests finally proved TUI had no EventID surface and terminal delivery could not report write failure before acknowledgement.
- **Issue identity**: The issue creation prompt requires the exact `<!-- fox-autodev-item-id:<ItemID> -->` body marker. Recorded numbers use `gh issue view` in the current repository and require that marker while accepting renamed and closed issues. Unbound lookup uses paginated, slurped GitHub search results and exact body-marker filtering; zero matches permits core creation, one binds, and multiple return `IssueIdentityConflictError` without retrying core work.
- **Durable publication**: The issue number and one pending deterministic `issue:<ItemID>:<number>` event enter the same candidate ledger commit. Reporting starts only after that commit, and PR intent starts only after successful adapter delivery and a separate durable delivery acknowledgement. Restart replays every pending event with the same identity before remote stages. Binding, outbox identity, append cardinality, delivery monotonicity, immutable issue binding, and deep-copy boundaries are validated.
- **Adapter contract**: Every Autodev `Reporter` consumes `RemoteEvent` and returns delivery error. Terminal and TUI reporters deduplicate successful delivery by EventID; failed writes, unsupported event kinds, and canceled TUI sends remain pending. Physical output can repeat across process crash, but logical identity remains stable and one process emits one observation.
- **Fixture corrections**: Multi-item GitHub fakes now retain issue numbers by marker and implement recorded-number reads. Their previous single global marker or arbitrary map entry caused later items to bind the first issue and made PR verification retry indefinitely once production stopped using titles.
- **Green gates**: complete `internal/autodev` and `internal/tui`; focused marker, outbox, restart, adapter-failure, and copy-isolation tests; combined race; `go vet ./...`; and the complete repository suite outside the restricted loopback-listener sandbox all PASS. `DV-AUT-007` is corrected and T110 is unblocked.

## T110 / D-AUT-008: Item-Owned Post-Run Lifecycle

- **Compile Red**: Corrected Autodev tests required `CoreRunner.Drain`, `CoreRunner.Close`, and typed `CoreLifecycleError`, none of which existed. After the control-plane contract was introduced, the real app adapter failed compilation because `AgentRunner` still offered only the unbounded, context-free `WaitForExtraction` convenience method.
- **Control-plane ordering**: StageMachine joins post-run work after every started core attempt, including error and cancellation outcomes, before ground-truth verification, Engineer review, retry, or another run. Drains use the active owner context when possible and a fresh two-minute-ceiling context after cancellation. A drain failure is terminal lifecycle evidence rather than a retryable verification gap.
- **Final ownership**: `processItem` owns exactly one final Close after lazy core construction. Every success path closes before done persistence, done reporting, worktree removal, and next-item selection. Every error path closes through a defer using `context.WithoutCancel` plus the same fixed ceiling. Close failure returns `CoreLifecycleError`; the item remains non-done and its worktree is retained.
- **Runtime cancellation**: `PerRunHooks.FireTrackedContext` combines the caller-owned lifecycle with the existing extraction timeout. `AgentRunner.DrainExtraction` serializes with Run and joins tracked work; `CloseExtraction` first cancels the extraction lifecycle and then joins. The app CoreRunner adapter delegates both operations directly.
- **Profile isolation**: `AgentRunnerConfig.ExtractionContext` is opt-in. Autodev factory binds it to the item parent; ordinary CLI, TUI, Feishu, and AgentOps construction retains the historical background extraction lifecycle. A canceled constructor call context therefore cannot accidentally disable fire-and-forget extraction in another profile.
- **Behavior evidence**: Scripted two-item cores fail immediately if a later run starts before drain or a later item starts before close. Blocking lifecycle tests prove no early worktree removal, bounded failure retention, and fresh contexts after parent cancellation. Real extraction-provider tests prove both explicit Close cancellation and parent-cancelled Drain join. Existing provider-error and panic tests retain warning-only, non-terminal behavior, and one-shot CLI still waits before exit.
- **Green gates**: complete affected Autodev, app, automemory, architecture, and Fox command packages; focused `DV-AUT-008`; combined race; `go vet ./...`; and the complete repository suite outside the established loopback-listener sandbox all PASS. `DV-AUT-008` is corrected and T111 is unblocked.

## T111 / D-AUT-009: Truthful Strict-Serial Configuration

- **Red evidence**: The retained proof accepted `parallel`, numeric-looking, future, whitespace-padded, and every other string without warning while execution always remained serial. Corrected tests required a typed rejection contract that production did not expose.
- **YAML domain**: `configFile` stores the concurrency value as a `yaml.Node`, preserving the distinction between omission and explicit empty/null as well as scalar kind and YAML tag. Only an omitted field or an exact case-sensitive string scalar `serial`, quoted or plain, is accepted. Numeric, null, collection, whitespace, case variant, parallel, future, and unknown values return `InvalidConcurrencyError` through the existing parse error chain.
- **Resolved boundary**: `Orchestrator.Run` validates `AutodevConfig.Concurrency` before ledger loading and before Git/GitHub preconditions. This prevents tests, future composition, or alternate adapters from bypassing the loader with an unsupported resolved value. Invalid configuration starts no external work and emits no warning fallback.
- **Compatibility**: Missing and empty config files retain the serial default; existing exact `concurrency: serial` files retain their behavior. The accepted execution proof still observes strict item start/done serialization.
- **Green gates**: focused `DV-AUT-009` and config tests; complete Autodev, app, and Fox packages; combined race; `go vet ./...`; and the complete repository suite outside the established loopback-listener sandbox all PASS. `DV-AUT-009` is corrected and T112 is unblocked.

## T112 / D-AUT-010: Correlated Core Outcome and Recovery Policy

- **Compile Red**: Corrected tests could not compile because `CoreRunner.Run` returned an independently nullable `*engine.RunResult` and `error`; no typed status, retry class, lifecycle evidence, attempt identity, durable retry record, or partial-only Engineer DTO existed. After introducing the outcome contract, the production orchestrator still failed the fresh-runner test because it passed a bare, non-replaceable runner.
- **Typed terminal contract**: One validated `CoreOutcome` distinguishes succeeded, failed, cancelled, turn-exhausted, and start-failed. `CoreOutcomeError` retains the complete outcome and unwraps the original cause. The app adapter observes `OnRunStart` plus committed `OnMessage` callbacks, never promotes an arbitrary error-path `RunResult.FinalMessage`, and preserves the downstream reporter's optional detailed/streaming capability surface exactly.
- **Attempt and retry authority**: Schema v3 adds append-only deterministic attempt/correlation history while loading v2 records without reinterpretation. A running intent commits before core invocation; after Drain and read-only Verify, exactly one terminal record retains status, session/run identity, retry class, and cause. Provider, retryable tool, and turn-limit outcomes use the same runner; conservative persistence/corruption failures require an item-owned replacement that closes the prior generation before constructing the next one.
- **Cancellation and presentation**: Every started outcome drains and receives verification. Cancellation reconciliation detaches only the bounded read-only Verify context, never retries or authorizes new work, and persists an already satisfied stage before still returning cancellation. Engineer review receives a typed evidence DTO and labels every non-success message as partial evidence rather than a final answer.
- **Green gates**: focused `DV-AUT-010`; complete Autodev and app packages; affected-package race; `go vet ./...`; and the complete repository suite outside the established loopback-listener sandbox all PASS. `DV-AUT-010` is corrected, all ten Autodev corrections are Green, and the T044 correction stop is cleared so T045 may proceed.
