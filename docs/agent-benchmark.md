# Agent Benchmark

This page records the local agent-investigation benchmark. The first recorded
run is a **pilot**: one repetition per scenario, one scenario at a time, because
the laptop GPU cannot run parallel investigations. It is evidence, not the
final matrix, and it contains mixed results.

## Protocol

- Target Profile: `deploy/targets/target-app/profile.yaml`.
- Nine fault scenarios with ground truth under
  `deploy/targets/target-app/scenarios/`, grouped by fault class in
  `deploy/targets/target-app/templates.yaml`.
- Paired arms on the same incident: `confined` (Region Scope enforced by the
  MCP servers, audited one-hop expansion) and `unconfined` (full observed
  architecture).
- Same model, prompt skeleton, tools, and caps in both arms.
- Model: `qwen3.5:4b`, thinking enabled, temperature 0, `num_ctx` 32768.
- Caps: 20 tool calls, 160,000 total tokens, 10 minutes wall time.
- Evidence baseline: 5 minutes, set through the history chart values for
  benchmark work; the product chart default stays 15 minutes.
- Scenarios are run one at a time; both arms of one incident run sequentially.

## Pilot result (2026-11-15, one repetition per scenario)

| scenario | arm | root-cause match | route match | failure-mode match | tokens | calls | denials | expansions | artifact dir |
| --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | --- |
| catalog-http-500 | confined | no (`gateway`) | yes | no | 146,701 | 20 | 2 | 0 | `pilot-20261115T164010Z` |
| catalog-http-500 | unconfined | **yes** (`catalog`) | yes | no | 97,977 | 20 | 0 | 0 | `pilot-20261115T164010Z` |
| auth-login-latency | confined | **yes** (`auth`) | no | no | 146,227 | 14 | 0 | 0 | `pilot-rest` |
| auth-login-latency | unconfined | no (`payment`) | no | no | 92,706 | 20 | 0 | 0 | `pilot-rest` |
| external-payment-outage | confined | no (`payment`) | yes | yes | 63,628 | 9 | 0 | 0 | `pilot-external` |
| external-payment-outage | unconfined | no (`payment`) | yes | no | 59,025 | 15 | 0 | 0 | `pilot-external` |
| postgres-outage | confined | no (`orders`) | yes | no | 141,032 | 16 | 0 | 0 | `pilot-postgres` |
| postgres-outage | unconfined | no (`orders`) | yes | no | 143,863 | 19 | 0 | 0 | `pilot-postgres` |
| catalog-crashloop | confined | no (`gateway`) | yes | no | 73,016 | 20 | 0 | 1 | `pilot-crashloop` |
| catalog-crashloop | unconfined | **yes** (`catalog`) | yes | no | 95,381 | 19 | 0 | 0 | `pilot-crashloop` |

Region recall: catalog `true`, auth `true`, external `false`, postgres `false`,
crashloop `false`. The dependency scenarios intentionally start with the root
cause outside the initial region; only the crashloop confined run performed an
expansion, and it still guessed `gateway`.

## Fixes found by running the pilot

- Auth latency fixture: `HTTPChaos.delay` is a duration string, not an object.
- Harness `kubectl` calls now carry the profile namespace; the first
  external/postgres `scale_zero` attempts targeted the default namespace.
- External and Postgres outage scenarios use `scale_zero` instead of pod
  deletion, which Deployment/StatefulSet controllers immediately undo.
- Crash-loop image swap now scales the old catalog pod to zero first; a rolling
  update with one replica kept the old healthy pod serving and produced no
  outage.
- Detected incidents interrupted by a history restart are closed with their
  stored window instead of remaining permanently open.
- Incident JSON decoding in the harness now uses explicit JSON tags.
- The harness reserves token budget for a final diagnosis and asks for one final
  answer when the tool-call cap is reached.

## Interpretation

- The 4B model can drive the tool loop but is not reliable at the final
  root-cause classification: two of five scenarios had one correct arm, three
  had none, and the correct failure mode was rare.
- Confinement did not produce a token advantage: confined runs used more tokens
  in four of five scenarios and performed one expansion (crashloop).
- The confined external and Postgres runs did not expand at all, so they could
  not reach the dependency root cause. The crashloop run expanded but still
  answered `gateway`.
- Catalog confined had two real scope denials; inspect the transcript before
  drawing conclusions from that run.
- These are pilot numbers, not repeated measurements. Do not quote them as a
  benchmark result or claim statistical significance.

## Not run

- The 9-scenario one-repetition coverage pilot for the four new scenarios.
- The full matrix: nine scenarios × three repetitions × two arms.
- A healthy negative-control episode in the harness.
- The final consolidated report with per-scenario medians/ranges and per-class
  coverage.

## Reproduce

```bash
./scripts/agent-benchmark.sh --pilot   # first three scenarios, one repetition
./scripts/agent-benchmark.sh --full    # nine scenarios, three repetitions
```

The harness requires steady Simulator traffic on the Target App, a reachable
Weave API on `:8090`, and the Target App port-forward. It sets the 5-minute
benchmark baseline, applies the read-only RBAC, and mints a read-only kubeconfig
for the MCP servers. Run one scenario at a time if GPU memory is limited.



## Fault-class taxonomy

The Target Profile declares seven service-level fault classes. Four are covered
by the nine scenarios; three are declared as coming soon and are not yet
benchmarked. Classes are design patterns, not fixtures: a scenario names one
class in its `template` field. This taxonomy is service-level only; it does not
inject code-level defects, mutate source, or edit configuration.

| class | status | scenarios |
| --- | --- | --- |
| internal logic fault | covered | catalog-http-500, auth-login-latency |
| external dependency handling | covered | external-payment-outage, postgres-outage |
| deployment manifest error | covered | catalog-crashloop, orders-replicas-zero, payment-image-pull-backoff |
| resource and infrastructure fault | covered | catalog-cpu-stress, catalog-network-loss |
| data contract mismatch | coming soon | — |
| resource identity mismatch | coming soon | — |
| configuration mismatch | coming soon | — |

Coverage claim: **4 of 7 classes, at service level**. The coming-soon classes
need fault machinery the project does not have today (source mutations,
manifest or configuration editing, protocol-level schema changes). The
benchmark report must show them rather than quietly omitting them.

The four new scenarios were added with the taxonomy, but have not yet been
through the hosted-agent matrix. Their ground truth, detection rules, and
fixtures still need the one-repetition coverage pilot described below.

## Hosted DSH agent run (2026-11-15)

Strategy changed after the 4B local-model pilot: use a fresh DSH subagent
(`deepseek-flash`, reasoning effort max) as the investigator, with no GPU and
no inherited conversation. The five MCP servers remain the tool boundary. A new
`weave-tool` CLI starts the matching server with the same `WEAVE_SCOPE_FILE`,
so the confined arm is still enforced by the MCP servers. Each subagent received
only a task file and the arm wrapper; ground truth, reports, specs, and `.git`
were hidden during the runs. Child session logs were audited: all investigation
commands went through the wrapper. Tokens are the DSH session usage totals
(including cache reads); duration is session creation to the last event.

| scenario | arm | root cause | route | mode | duration | input | output | cache read | total tokens | tool calls | expanded |
| --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| catalog-http-500 | confined | ✅ | ✅ | ✅ | 168.3s | 41,191 | 27,227 | 552,576 | 620,994 | 23 | no |
| catalog-http-500 | unconfined | ✅ | ✅ | ✅ | 126.9s | 55,332 | 21,113 | 548,864 | 625,309 | 27 | no |
| auth-login-latency | confined | ✅ | ❌ | ✅ | 75.4s | 22,971 | 11,107 | 417,536 | 451,614 | 27 | no |
| auth-login-latency | unconfined | ✅ | ❌ | ✅ | 107.4s | 92,140 | 18,302 | 723,968 | 834,410 | 30 | no |
| external-payment-outage | confined | ✅ | ✅ | ✅ | 58.8s | 37,799 | 9,147 | 441,088 | 488,034 | 25 | yes |
| external-payment-outage | unconfined | ✅ | ✅ | ✅ | 156.4s | 33,498 | 25,528 | 692,992 | 752,018 | 29 | no |
| postgres-outage | confined | ✅ | ✅ | ✅ | 58.1s | 28,567 | 9,268 | 365,568 | 403,403 | 30 | yes |
| postgres-outage | unconfined | ✅ | ✅ | ✅ | 100.5s | 48,102 | 17,224 | 370,432 | 435,758 | 16 | no |
| catalog-crashloop | confined | ✅ | ✅ | ✅ | 64.8s | 26,080 | 9,876 | 404,864 | 440,820 | 16 | no |
| catalog-crashloop | unconfined | ✅ | ✅ | ✅ | 65.4s | 26,536 | 11,075 | 225,024 | 262,635 | 25 | no |

Hosted result: **10/10 root cause, 10/10 failure mode, 8/10 route.** Both arms
were correct on every scenario. The two route misses are the auth scenario:
the injected slow route is `POST /api/auth/login`, while the observed 5xx
breach appears on `POST /api/payments` because the gateway synchronously calls
auth before protected routes. That is a ground-truth ambiguity rather than a
wrong root cause.

The confined hosted runs correctly used `expand_scope` for the external and
Postgres dependency outages; the 4B confined runs did not. The hosted model
also correctly distinguished the crash-loop bad deployment from a healthy
catalog rollback using Kubernetes events and rollout history.

Raw records: `reports/agent-benchmark/dsh-pilot-20261115/records.json`.
