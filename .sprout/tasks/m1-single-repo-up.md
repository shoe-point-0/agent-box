<!-- sprout-task
{"schema_version":1,"id":"m1-single-repo-up","status":"open","created_at":"2026-08-29T06:48:36.203491553Z"}
-->

# Deliver the single-repository up vertical slice

Execute this task end to end. Inspect before editing, keep scope tight, update this packet as work progresses, and do not claim completion without recorded verification evidence.

## Goal

Deliver the single-repository up vertical slice

## Context

- Milestone 1. Dependencies: m1-machine-preflight, m1-git-worktrees, m1-lima-adapter, m1-filesystem-verifier, m1-compose-policy-core, m1-compose-adapter, m1-context-document, m1-reconcile-engine, m1-daemon-api, m1-cli-foundation. Source: THE_BEGINNING.md Sections 9.3, 14.1, and Milestone 1.

## Requirements

- Wire lease, validation, Git, Plan, runtime host, mounts, Compose, basic readiness, context, and READY transition for one local repository.
- Re-observe each boundary and resume matching partial resources.
- Return success only after the declared workspace service is usable.

## Constraints

- Do not commit, push, or mutate shared infrastructure as part of this task.
- Support claims require real execution evidence on the claimed platform.
- Final Darwin hardware evidence input is required for the remaining cold/warm `abox up demo` and detached-client acceptance criteria; the Linux lane must not repeatedly select this packet after the verified implementation has reached the real Apple Silicon VZ hardware gate.

## Acceptance criteria

- [ ] A clean fixture reaches READY from zero with abox up demo.
- [ ] A second up is a measured no-op with no duplicate worktree, VM, network, volume, container, or setup effect.
- [ ] Killing the CLI does not kill or duplicate daemon-owned reconciliation.

## Technical approach

- Add a focused `internal/up` coordinator above the existing daemon journal and
  reconcile engine. The daemon operation context, rather than the attached CLI
  request, owns the complete attempt. Acquire and periodically renew one fenced
  Box lease around every DAG execution, cancel mutation immediately if renewal
  is lost, and release only the exact still-owned fence through a bounded
  cleanup context.
- Assemble the existing validation, repository/private-store/worktree, Plan,
  Lima, filesystem, Compose, readiness, context, state, and removal-snapshot
  boundaries as independently observable reconcile operations. Keep provider
  facts inside each operation and journal only stable operation keys and
  created/no-op transitions.
- Make READY publication the final operation, after live workspace-service and
  filesystem readiness. Atomically retain the Plan/current generation, cached
  component observations, context identity, and complete removal ownership;
  never infer external existence from SQLite on a later run.
- Prove the control-plane contract first with real SQLite/journal execution and
  deterministic stage fixtures: cold execution mutates each stage once, warm
  execution re-observes every stage without mutation, lease loss cancels the
  DAG, and detaching the client leaves daemon-owned execution running. Then
  assemble the unchanged concrete adapters and exercise the disposable
  single-repository smoke fixture without claiming unavailable Lima/VZ support.

## Execution checklist

- [x] Inspect the relevant code, tests, and repository instructions.
- [x] Refine the technical approach and acceptance criteria in this task before editing.
- [ ] Implement the smallest complete change.
- [ ] Run every verification item and address failures.
- [ ] Record concrete validation evidence and completion notes in this task.

## Verification

- [ ] go test ./...
- [ ] go test -race ./...
- [ ] go vet ./...
- [ ] Run the real single-repository cold and warm up smoke test

## Validation evidence

- Control-plane slice: `go test -race -count=20 ./internal/up` passed. The new
  coordinator executed a nine-stage dependency chain under the real
  SQLite-backed journal lease, re-observed all nine stages on a second attempt,
  and recorded zero second-attempt applies. Every stage applied exactly once
  across cold and warm execution.
- The focused suite held a 60 ms lease across a 90 ms operation with at least
  three successful renewals and one exact-fence release. Injected renewal loss
  canceled the executor and performed zero releases because ownership could no
  longer be proven.
- `go vet ./internal/up`, `make lint` (`0 issues`), and `git diff --check`
  passed for the control-plane slice.
- READY-state slice: `go test -race -count=20 ./internal/state` passed after
  adding schema v4 and `Store.PublishReady`. The transaction commits the
  immutable Plan/current generation, context path and digest, freshly observed
  repository/worktree, Lima and Compose caches, and the complete removal
  ownership snapshot together. Repeating an identical publication is a no-op;
  attempts to rebind one generation are refused.
- The READY-state suite injected a unique Git-store cache collision after the
  generation insert point. SQLite rolled back the generation, repository, and
  removal-authority rows and left the Box at generation zero in `planning`.
- Post-slice `make check` passed: lint reported `0 issues`, module verification,
  all unit and race tests, vet, builds, smoke, and automation tests were green.
- Attempt-preparation slice: `go test -race ./internal/up ./internal/state`,
  `go vet ./internal/up ./internal/state`, and `git diff --check` passed. The
  daemon can now reconstruct the selected Box intent from SQLite after request
  detachment, recompile the immutable Plan through the read-only planning
  boundary, derive only centralized Box paths, and select generation 1 only
  when no current generation exists. An exact warm Plan reuses its persisted
  generation and complete removal authority; a current generation with missing
  or drifted authority fails closed with a stable ownership/identity conflict.
- Post-preparation `make check` passed: lint reported `0 issues`; module
  verification, all unit and race tests, vet, builds, smoke, shell checks, and
  Python automation tests were green. `sprout check m1-single-repo-up` passed.
- Partial Git-authority slice: schema v5 adds one generation-scoped,
  single-repository recovery record tied to an actual daemon `up` operation.
  `SaveReconciliationGitAuthority` records intent before apply and permits only
  monotonic addition of the exact private-store identity and locked-worktree
  identity. A later daemon operation can continue the same immutable claim;
  drift, proof replacement, and proof regression fail closed.
- `go test -race ./internal/state`, `go vet ./internal/state`, and
  `git diff --check` passed for the partial-authority slice. The tests exercise
  claim/store/worktree progression, cross-operation recovery, drift and
  regression refusal, schema migration from every retained fixture, and atomic
  retirement of matching partial authority during READY publication.
- Post-authority `make check` passed: lint reported `0 issues`; module
  verification, all unit and race tests, vet, builds, smoke, shell checks, and
  Python automation tests were green. `sprout check m1-single-repo-up` passed.
- Concrete Git-reconcile slice: `internal/up.ReconcileGit` now consumes the
  immutable `PreparedAttempt` with the production `state.Store`,
  `gitstore.Materializer`, and `worktree.Manager` contracts. It persists or
  transfers the exact generation claim before mutation, persists the private
  store inode immediately after materialization, and persists the locked
  worktree inode and Git registration immediately after creation.
- `go test -race -count=10 ./internal/up` passed against real Git. The test
  interrupted reconciliation after private-store persistence, resumed under a
  later operation without a second clone, then re-observed the retained locked
  worktree without a second add or lock. Replacing the retained store inode at
  its deterministic path was refused and the foreign marker remained intact;
  drifted recovery authority was refused before any external adapter call.
- `go vet ./internal/up` and `git diff --check` passed. Post-slice `make check`
  passed: lint reported `0 issues`; all unit and race tests, vet, builds, smoke,
  shell checks, and Python automation tests were green.
- This evidence does not yet satisfy the cold/warm Box acceptance criteria: the
  prepared attempt is not yet consumed by concrete Lima, filesystem, Compose,
  context, and READY operations in `cmd/aboxd`. The final READY
  transaction exists but is not wired to the filesystem readiness gate, and no
  runtime-host support claim is made.
- Generated-Plan slice: schema v6 adds generation-scoped partial authority for
  the generated Box root. `internal/up.ReconcileGeneratedPlan` writes the
  immutable generation claim before filesystem mutation, retains the root
  device/inode before writing `plan.json`, and resumes only after a fresh path
  proof. Existing unclaimed roots, substituted root inodes, symlinked parent
  paths, unsafe Plan files, and changed Plan content are refused rather than
  adopted or overwritten.
- READY publication now compares any partial generated-root authority with the
  complete removal snapshot in the same transaction before retiring it. An
  injected mismatch rolls back publication and leaves the recovery authority
  intact. Migration tests pass from every schema fixture through schema v6.
- `go test -race -count=10 ./internal/state ./internal/up`,
  `go vet ./internal/state ./internal/up`, and `git diff --check` passed. The
  focused up test interrupted after root authority persistence, resumed without
  replacing the root, and measured a third reconciliation with zero additional
  Plan writes. Post-slice `make check` passed with lint reporting `0 issues`,
  all unit and race tests, vet, builds, smoke, shell, and automation checks
  green; `sprout check --all` passed.
- This slice still does not satisfy cold/warm Box acceptance: the generated
  Plan and concrete Git reconciler are not yet assembled into the production
  DAG, and Lima onward remains unwired.
- Partial Lima-authority slice: schema v7 adds one generation-scoped recovery
  record for the exact Lima artifact, immutable runtime-host Spec, generated
  config path, config path-set proof, and created instance identity. Intent is
  durable before either config or instance proof; proofs can only advance and
  can transfer to a later running daemon operation without changing identity.
- READY publication now compares complete partial Lima authority with the Lima
  removal snapshot and centralized `lima.yaml` path in the same transaction.
  Exact authority is retired; an artifact mismatch rolls back publication and
  preserves the recovery record.
- `go test -race -count=10 ./internal/state`, `go vet ./internal/state`, and
  `git diff --check` passed. Tests cover claim/config/instance progression,
  cross-operation recovery, drift, regression, proof replacement, matching
  READY retirement, mismatch rollback, and migration from every schema fixture
  through schema v7.
- Post-slice `make check` passed with lint reporting `0 issues`; module
  verification, all unit and race tests, vet, builds, smoke, shell, and Python
  automation checks were green.
- This slice does not create or start a VM. The concrete Lima reconciler and
  production DAG remain unwired, so no Linux or Darwin runtime-host support
  claim is made.
- Concrete Lima-reconcile slice: `internal/up.ReconcileLima` now consumes the
  prepared immutable attempt, retained generated-root proof, production
  `state.Store` authority contract, and concrete `lima.Adapter` lifecycle
  contract. It persists intent before config creation, retains the exact config
  path-set before create, retains live embedded instance identity before start,
  and freshly proves retained config and instance identity on every resume.
- `go test -race -count=20 ./internal/up` passed. The focused fixture interrupted
  first after config creation and then after instance creation, resumed the
  exact resources under later operations, and measured one config write, one
  instance create mutation, and one start mutation across subsequent warm
  reconciliation. Replacing the retained config inode was refused before any
  runtime call and the foreign file remained unchanged.
- `go vet ./internal/up`, `git diff --check`, `sprout check
  m1-single-repo-up`, and `sprout check --all` passed. Post-slice `make check`
  passed with lint reporting `0 issues`; module verification, all unit and race
  tests, vet, builds, smoke, shell, and Python automation checks were green.
- That concrete-Lima evidence was contract-fixture evidence, not a real VM
  claim. At that point the three concrete stages were not yet assembled and
  filesystem readiness through READY remained unwired.
- Foundation graph-prefix slice: `internal/up.ReconcileFoundation` now composes
  the concrete generated Plan, Git, and Lima reconcilers in explicit dependency
  order. The Lima intent builder runs only after the immutable Plan, private
  store, and linked worktree exist, so it can record fresh mount path evidence.
  A later-stage failure deliberately retains earlier partial authority for
  resume instead of compensating by deleting owned source or runtime inputs.
- `go test -race -count=10 ./internal/up` passed. The integration fixture used
  real Git for private-store/worktree creation, interrupted after Git authority
  was complete but before Lima intent, then resumed and ran a third warm pass.
  Across all attempts it measured one Plan write, one private-store mutation,
  one worktree creation, one runtime-fixture create mutation, and one
  runtime-fixture start mutation;
  every warm boundary was freshly re-observed.
- `go vet ./internal/up`, `git diff --check`, and post-slice `make check` passed;
  lint reported `0 issues`, and all unit/race tests, vet, builds, smoke, shell,
  and Python automation checks were green.
- This graph prefix is not yet dispatched by `cmd/aboxd` and does not make a
  real VM support claim. Filesystem verification, Compose, readiness, context,
  READY publication, daemon assembly, and the cold/warm CLI smoke remain open.
- Compose-input slice: `internal/up.ReconcileComposeInputs` now consumes the
  freshly proven generated-root, Git, and running-Lima results rather than
  cached component rows. It refuses substituted Git/runtime paths before the
  normalization runner is invoked, compiles the repository include plus Agent
  Box policy, and records the complete path set consumed by runtime apply and
  filesystem readiness.
- Warm compilation still reruns the pinned version and both normalization
  boundaries, but exact generated root, override, and redacted-model bytes now
  preserve their existing inodes. `go test -race -count=10 ./internal/compose
  ./internal/up` passed: twenty compiler passes and the assembled cold/warm
  fixture retained all three file identities; replacing the observed worktree
  was refused with zero additional normalization calls.
- `go vet ./internal/up ./internal/compose`, `git diff --check`, `sprout check
  m1-single-repo-up`, and `sprout check --all` passed. Post-slice `make check`
  passed with lint reporting `0 issues`; module verification, all unit/race
  tests, vet, builds, smoke, shell, and Python automation checks were green.
- The prior next-step hypothesis of invoking filesystem readiness immediately
  after Lima was wrong: the existing readiness contract requires a freshly
  applied `compose.RuntimeSpec`, immutable container IDs, Docker mount
  observations, and generated Compose path evidence. Compose compilation and
  resource apply must therefore precede filesystem verification; bypassing
  that order would turn generated intent into fake live evidence.
- Partial Compose-authority slice: schema v8 adds one generation-scoped
  recovery record for the exact `compose.RuntimeSpec` consumed by apply and the
  resource identities observed afterward. Intent is durable before Compose may
  mutate; container, network, and volume proof can only be added monotonically
  or transferred unchanged to a later running daemon operation.
- READY publication now compares complete partial Compose authority with the
  Compose removal specification and resource records in the same transaction.
  Exact authority is retired; a mismatched runtime ID rolls back publication
  and leaves the recovery record intact. Migration tests pass from every
  retained schema fixture through schema v8.
- `go test -race -count=10 ./internal/state`, `go vet ./internal/state`, and
  `git diff --check` passed. Post-slice `make check` passed with lint reporting
  `0 issues`; module verification, all unit/race tests, vet, builds, smoke,
  shell, and Python automation checks were green.
- This slice deliberately does not call `compose.Runtime.Apply`: durable intent
  now exists for crash recovery, but the concrete reconciler must still apply,
  freshly observe, retain immutable IDs, and prove warm identity before
  filesystem readiness can consume the result.
- Concrete Compose-reconcile slice: `internal/up.ReconcileCompose` now consumes
  the compiled `compose.RuntimeSpec` through the production `state.Store` and
  `compose.Runtime` contracts. It persists or transfers intent before apply,
  retains the exact container, network, and volume IDs observed afterward, and
  uses observation rather than apply whenever complete recovery authority
  already exists.
- `go test -race -count=10 ./internal/up` passed. The focused fixture proved
  intent preceded runtime mutation, one cold mutation survived a failed record
  persistence followed by an idempotent retry, retained authority transferred
  across operations, and later attempts re-observed without apply. READY-derived
  authority also took the observation-only path, while a replaced live
  container ID was refused before any mutation.
- `go test -race -count=10 ./internal/compose ./internal/state`, `go vet
  ./internal/up`, `make lint` (`0 issues`), and `git diff --check` passed.
  Post-slice `make check` passed with module verification, all unit/race tests,
  vet, builds, smoke, shell, and Python automation checks green.
- This remains contract-fixture evidence rather than a real Docker-in-Lima
  apply. Filesystem/workspace usability, context publication, atomic READY,
  daemon dispatch, and the real cold/warm CLI smoke remain open.
- Filesystem-readiness wiring slice: `internal/up.ReconcileFilesystem` now
  consumes only a complete `ComposeResult`, passes its immutable runtime Spec
  into the existing live readiness gate, and keeps the context/READY publisher
  inside that gate's final path-identity boundary. The earlier post-apply
  observation must match retained IDs before collection starts, and the fresh
  Docker observation collected with Lima, guest-kernel, container-kernel, and
  workspace-service evidence must match the same complete ID set before the
  publisher can run.
- `go test -race -count=10 ./internal/up` and `go test -race -count=10
  ./internal/filesystem` passed. The coordinator tests proved that stale input
  is refused before the gate, changed fresh container identity cannot publish,
  and the publisher receives the gate's fresh observation rather than the
  earlier Compose view. The filesystem suite exercised the concrete mount-set,
  undeclared-mount canary, bidirectional byte/mode/rename/delete, native watcher,
  immutable-container, cleanup, and final path-substitution checks.
- `go vet ./internal/up ./internal/filesystem`, `git diff --check`, `sprout
  check m1-single-repo-up`, and `sprout check --all` passed. Post-slice `make
  check` passed with lint reporting `0 issues`; module verification, all
  unit/race tests, vet, builds, smoke, shell, and Python automation checks were
  green.
- This is still deterministic adapter evidence, not a real Lima/Compose smoke
  claim. Context materialization and `Store.PublishReady` must run in the
  readiness callback before production daemon dispatch can honestly return
  success.
- READY-publication slice: `internal/up.ReconcileReady` now runs context
  generation and the production `state.Store.PublishReady` transaction only
  inside `ReconcileFilesystem`'s final callback. It derives repository and
  service context from the freshly proven foundation and Docker observations,
  rejects stopped, unhealthy, duplicate, or missing workspace services, and
  assembles complete generated-root, Git, Lima, Compose, and path removal
  authority without consulting cached component rows.
- The context path is materialized mode `0600`, read back, and hashed before the
  state transaction. Exact warm evidence preserves the existing context inode;
  differing retained bytes are refused without replacement. The generated Lima
  config path and content are freshly reproved before its digest enters READY.
- `go test -race -count=10 ./internal/up` passed. The focused integration test
  wrote a real context document, committed READY through a real SQLite store,
  loaded the retained removal snapshot, and repeated publication without
  replacing the context inode. Injected filesystem, context-redaction, retained
  context, and READY-transaction failures did not publish state; a failed state
  transaction left the complete context available for an exact recovery retry.
- `go vet ./internal/up`, `make lint` (`0 issues`), `git diff --check`, `sprout
  check m1-single-repo-up`, and `sprout check --all` passed for this slice.
- This evidence remains a deterministic Linux fixture, not a real Lima VM or
  Docker-in-Lima claim. The complete runner is not yet dispatched by
  `cmd/aboxd`, and the cold/warm CLI and client-detachment acceptance criteria
  remain open.
- Complete-runner slice: `internal/up.Pipeline` now executes preparation,
  generated/Git/Lima foundation reconciliation, Compose compilation, Compose
  apply/observation, filesystem/context readiness, and atomic READY publication
  in one dependency-ordered call. `AssemblePipelineCoordinator` places that
  entire call inside the existing fenced daemon lease, so preparation and every
  mutation run under one daemon-owned cancellation lifetime.
- Runtime-specific READY adapter construction is deliberately delayed until the
  fresh `compose.RuntimeSpec` exists. `go test -race -count=10 ./internal/up`
  passed: the focused runner tests proved exact stage order, fresh-result
  forwarding, and immediate stop at each injected boundary failure. `go vet
  ./internal/up` and `git diff --check` passed.
- Post-slice `make check` passed with lint reporting `0 issues`; module
  verification, all unit/race tests, vet, builds, smoke, shell, and Python
  automation checks were green. `sprout check m1-single-repo-up` and `sprout
  check --all` passed.
- The pipeline is not yet instantiated by `cmd/aboxd`; production artifact,
  Lima, Compose, and filesystem adapter assembly plus the real cold/warm and
  detached-client smoke remain open.
- Production-assembly prerequisite slice: `internal/up.Pipeline` now delays
  `ComposeConfig` construction until the generated/Git/Lima foundation has
  freshly converged. The previous static `ComposeConfig` shape could not be
  assembled honestly: its normalization runner must target the just-proven
  Lima Spec, which does not exist when a per-operation pipeline is constructed.
  Capturing mutable runtime state in a shared runner would also make concurrent
  Box operations race and potentially execute against the wrong VM.
- `go test -race -count=10 ./internal/up`, `go vet ./internal/up`, and `git
  diff --check` passed. The focused builder test proves the delayed callback
  receives the exact prepared attempt and fresh foundation result and that a
  binding failure stops before Compose compilation.
- Post-slice `make check` passed with lint reporting `0 issues`; module
  verification, all unit/race tests, vet, builds, smoke, shell, and Python
  automation checks were green. `sprout check m1-single-repo-up` and `sprout
  check --all` passed.
- Inspection also rejected a tempting but unsafe production shortcut: the
  current `lima.Adapter.Invoke` boundary has no structured environment or
  secret channel, while `compose.Runner` requires both. Encoding Compose
  environment or secret values into guest argv would violate the project's
  no-secret-in-argv contract. Production dispatch therefore still requires a
  bounded runtime-host Compose command adapter (and the verified guest
  toolchain/environment contract it consumes), not an argv wrapper in
  `cmd/aboxd`.
- Runtime-host command slice: the bounded process and verified-tool execution
  boundaries now carry an owned stdin byte payload. `lima.Adapter.InvokeCommand`
  uses that channel to provide a deterministic, size-limited complete child
  environment to a clean `/usr/bin/env -i /bin/sh` bootstrap while keeping
  every environment and secret value out of both `limactl` and guest bootstrap
  argv. It rejects invalid environment names, NULs, oversized transport, and
  any registered secret found in argv or the guest working directory.
- The transport mechanism was checked against Lima v2.0.0's `shell.go`: its
  SSH subprocess explicitly forwards `os.Stdin`, while command arguments are
  assembled into the remote login-shell command. The implementation therefore
  sends sensitive values only over stdin and keeps the fixed bootstrap and
  workload arguments in the command channel.
- `compose.GuestRunner` maps compiler environment and redaction registrations
  onto that typed Lima command. `up.NewComposeConfigBuilder` constructs a new
  runner from each fresh `FoundationResult.Lima.Spec`; a focused test bound two
  calls through one shared builder to two different instance names without
  retaining mutable Box identity.
- `go test -race -count=10 ./internal/process ./internal/toolchain
  ./internal/lima ./internal/compose ./internal/up` passed. The tests exercised
  literal stdin including NUL bytes, artifact-layer forwarding, exact and
  ambient-free shell environment behavior, secret redaction and argv refusal,
  deterministic bounded script construction, partial result/error forwarding,
  and per-attempt runtime binding. `go vet` for the same packages, `make lint`
  (`0 issues`), and `git diff --check` passed. Post-slice `make check` passed
  with module verification, all unit/race tests, vet, builds, smoke, shell, and
  Python automation checks green.
- Guest-toolchain pipeline slice: `internal/up.Pipeline` now inserts live guest
  provisioning after the running Lima host and before Compose normalization.
  The stage requires the exact freshly observed `lima.Spec`, reruns the complete
  idempotent `guesttoolchain.Provisioner` contract on every cold or warm
  attempt, and passes its verified guest paths to both delayed Compose and READY
  adapter builders. Compose construction refuses a Docker path or runtime-host
  identity that differs from the verified guest result, so repository-owned
  commands cannot run through an unverified executable path.
- `go test -race -count=10 ./internal/up` passed and proved the guest boundary
  executes in dependency order, reruns on every attempt, stops the pipeline on
  failure, and rejects runtime-host drift before provisioning. Focused `go vet`
  and `git diff --check` passed. Post-slice `make check` passed with lint
  reporting `0 issues`; module verification, all unit/race tests, vet, builds,
  smoke, shell, and Python automation checks were green.
- Live prerequisite inspection found KVM available and Docker/Compose
  `29.6.1`/`5.3.0`, but no `limactl` executable and no guest image artifact in
  the repository. No VM was created. Production assembly must consume an
  explicit verified artifact installation; silently substituting the Machine
  Docker binary or an implicit Lima image would violate the existing toolchain
  and guest provisioning contracts.
- Production-dispatch slice: `internal/installation.Load` now consumes bounded,
  strict `abox.installation/v1alpha1` metadata and verifies the complete local
  Lima, guest image, Docker, Compose, and required utility set through the
  existing opaque toolchain contract. It performs no discovery, download, or
  installation. Guest paths are explicit, utility artifact/path sets must match,
  and the guest image path passed to Lima is canonicalized only after digest
  verification.
- `aboxd` accepts `--installation`, defaulting beside `state.db`, and uses that
  one installation contract for Plan tool versions, up, and removal. The
  production `up` dispatch now assembles the concrete private-store/worktree,
  Lima, guest-toolchain, Compose, filesystem, context, READY, lease, and journal
  adapters per operation. The previous host Docker/Compose/implicit `limactl`
  probes were removed; missing metadata returns `ABOX_PREREQUISITE_FAILED`
  through the real daemon API instead of `ABOX_FEATURE_NOT_SUPPORTED`.
- The VZ-only adapter is refused with `ABOX_MACHINE_UNSUPPORTED` during
  operation assembly on Linux, before generated-state or Git mutation. Fresh
  inspection on this lane reported Linux `7.0.0-28-generic` x86_64 with KVM,
  but no `limactl` and no default installation metadata at
  `/home/shoecifer/.local/state/agent-box/installation.json`; no VM or Box
  resource was created and no Linux runtime support is claimed.
- `go test -race -count=10 ./internal/installation ./cmd/aboxd` passed. The
  installation suite covered strict/trailing JSON rejection, schema and path
  validation, required utility-set equality, digest mismatch classification,
  symlinked metadata refusal, cancellation, and verified result isolation. The
  daemon suite exercised production `up` dispatch and its typed missing-
  installation failure. Focused `go vet` and `git diff --check` passed.
- Post-slice `make check` passed with lint reporting `0 issues`; module
  verification, all unit/race tests, vet, builds, smoke, shell, and Python
  automation checks were green. `sprout check m1-single-repo-up` and `sprout
  check --all` passed.
- Live-Machine preflight slice: `internal/machine.LocalObserver` now collects
  the current OS/architecture/accelerator tuple, canonical path ancestry and
  filesystem capacity, and a temporary IPv4 loopback bind without creating a
  directory or retaining a listener. Darwin uses Apple's documented
  `kern.hv_support` signal before reporting the VZ accelerator; non-Darwin
  Machines remain the unsupported QEMU engineering lane.
- Production `up` re-verifies the installation's complete pinned artifact set,
  requires at least 1 GiB on the state filesystem and 16 MiB on the daemon
  runtime filesystem, and proves `127.0.0.1:7777` can be bound before Git lookup,
  generated Box state, lease acquisition, or pipeline mutation. The exact
  configured daemon runtime root is observed rather than reconstructing it from
  the application root.
- `go test -race -count=10 ./internal/machine ./internal/installation
  ./cmd/aboxd` passed. The live observer tests covered existing and absent paths,
  no directory creation, symlinked ancestry refusal, occupied router detection,
  listener release, copied configuration, and platform collection. A production
  dispatch test loaded a valid installation on Linux, returned
  `ABOX_MACHINE_UNSUPPORTED`, and proved the deterministic Box root remained
  absent.
- `go vet ./internal/machine ./internal/installation ./cmd/aboxd`,
  `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`, and `git diff --check`
  passed. Post-slice `make check` passed with lint reporting `0 issues`; module
  verification, all unit/race tests, vet, builds, smoke, shell, and Python
  automation checks were green. Cross-compilation is compilation evidence only;
  no Darwin runtime behavior is claimed.

## Outcome and follow-ups

The first coherent slice adds `internal/up` as the daemon-owned fenced lifetime
around one reconciliation DAG. It validates command input, emits stable start
and ready events, continuously renews the Box lease, cancels immediately on
lease loss, and releases only a still-proven fence through cancellation-proof
bounded cleanup. Errors cross the daemon boundary only as stable redacted
contract codes.

The second coherent slice adds one atomic READY publication boundary. It
retains context identity and complete removal ownership with the current Plan,
updates only cached component observations, enforces monotonic generations,
and supports exact warm retries without allowing immutable generation evidence
to drift.

The third coherent slice adds the read-only front half of production attempt
assembly. It deliberately refuses to infer ownership from deterministic names,
paths, or cached component rows. Cold preparation is allowed only at generation
zero; warm preparation requires the exact current-generation removal snapshot.

The fourth coherent slice adds durable, monotonic partial authority for the
private Git store and linked worktree. It is intentionally retired only inside
the same transaction that publishes complete READY removal authority, so a
failed publication retains the recovery record and a successful publication
cannot leave two competing authority sources.

The fifth coherent slice consumes that authority in the concrete private-store
and worktree path. Recovery is proof-bearing: a retained private-store inode is
freshly checked by the worktree boundary, and a retained worktree is observed
rather than ensured; a deterministic path without matching retained identity
is never adopted.

The sixth coherent slice materializes the immutable generated Plan under
durable root authority. A crash after root creation can resume only when the
retained inode still matches, and READY cannot silently replace that partial
authority with a different removal root.

The seventh coherent slice establishes the Lima create/start recovery ledger.
The generated config and runtime host now have one monotonic authority source
that READY must consume atomically rather than reconstructing from a name.

The eighth coherent slice consumes that ledger through the concrete Lima
lifecycle contract. Matching resources resume from fresh path, immutable
config, and embedded metadata observations; retained proof replacement fails
before mutation.

The ninth coherent slice compiles Compose only after that foundation has been
freshly proven. Re-normalization is mandatory on warm runs, while byte-identical
generated evidence is no longer rewritten and therefore keeps the path identity
that later readiness and removal checks consume.

The tenth coherent slice establishes the Compose apply recovery ledger. It
binds generated inputs and immutable runtime-host identity before mutation and
ensures READY cannot silently replace partial runtime ownership with a
different live resource set.

The eleventh coherent slice consumes that ledger through the concrete Compose
runtime contract. A cold attempt applies only behind durable intent; complete
partial or READY authority takes the observation-only path and must match the
entire freshly observed resource set exactly.

The twelfth coherent slice binds that retained runtime authority to the existing
filesystem readiness gate. The final callback sees only a freshly re-observed
Docker set that still matches every retained resource ID, after Machine,
runtime-host, container-kernel, and workspace-service checks have passed.

The thirteenth coherent slice completes the final publication boundary. Context
bytes and service readiness now come from the same fresh observation consumed
by the filesystem gate, while one SQLite transaction promotes the generation
and retires only matching partial authority.

Next, use an explicit pinned installation plus the unchanged single-repository
fixture on a supported Darwin/arm64 VZ Machine to prove real cold READY, measured
warm no-op, and CLI detachment. Keep this packet open until those three
acceptance criteria pass. When the Machine router is introduced in Milestone 3,
its already-owned listener must become explicit preflight evidence rather than
being mistaken for an unrelated occupied port.

## Original request

Implement deliver the single-repository up vertical slice for Agent Box according to the cited specification and dependencies.
