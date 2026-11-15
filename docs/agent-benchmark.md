# Agent Benchmark

This page records the local agent-investigation benchmark and its results. It
is evidence, not a target: the first pilot produced mixed results with the 4B
model and they are reported as they happened.

## Protocol

- Target Profile: `deploy/targets/target-app/profile.yaml`
- Five fault scenarios with ground truth under
  `deploy/targets/target-app/scenarios/`.
- Paired arms on the same incident: `confined` (Region Scope enforced by the
  MCP servers, audited one-hop expansion) and `unconfined` (full observed
  architecture).
- Same model, prompt skeleton, tools, and caps in both arms.
- Model: `qwen3.5:4b`, thinking enabled, temperature 0, `num_ctx` 32768.
- Caps: 20 tool calls, 160,000 total tokens, 10 minutes wall time.
- Evidence baseline: 5 minutes, set for the benchmark through the history
  chart values; the product default remains 15 minutes.
- Ground truth is never exposed to the agent.

## Pilot result (2026-11-15, one repetition)

The pilot was run as three separate one-scenario runs after fixing two fixture
defects discovered on the first attempt:

- the auth latency fixture used an object-shaped `delay`; the CRD expects a
  duration string;
- the benchmark harness did not pass the target namespace to `kubectl`, so the
  external/postgres `scale_zero` injections failed before they started.

| scenario | arm | status | root-cause match | route match | failure-mode match | tokens | tool calls | denials | expansions |
| --- | --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: |
| catalog-http-500 | confined | completed | no (`gateway`) | yes | no (`bad_deployment`) | 146,701 | 20 | 2 | 0 |
| catalog-http-500 | unconfined | completed | **yes** (`catalog`) | yes | no (`bad_deployment`) | 97,977 | 20 | 0 | 0 |
| auth-login-latency | confined | completed | **yes** (`auth`) | no | no (`dependency_unavailable`) | 146,227 | 14 | 0 | 0 |
| auth-login-latency | unconfined | completed | no (`payment`) | no | no (`http_error`) | 92,706 | 20 | 0 | 0 |
| external-payment-outage | confined | completed | no (`payment`) | yes | yes | 63,628 | 9 | 0 | 0 |
| external-payment-outage | unconfined | completed | no (`payment`) | yes | no (`http_error`) | 59,025 | 15 | 0 | 0 |

Region recall: catalog `true`, auth `true`, external `false` (the root cause is
one hop outside the initial region, as expected for that scenario). The
confined external run did **not** perform the expected scope expansion and
therefore did not reach `external`.

Artifacts:

- `reports/agent-benchmark/pilot-20261115T164010Z/` — catalog scenario plus the
  first pilot manifest.
- `reports/agent-benchmark/pilot-rest/` — auth scenario rerun.
- `reports/agent-benchmark/pilot-external/` — external scenario rerun.

## Interpretation

- The 4B model is capable of the tool loop but not yet reliable at the final
  root-cause classification. One arm or the other was correct in two of three
  scenarios; both were wrong in the external scenario.
- Confinement did not produce the expected token advantage in this pilot:
  confined runs used more tokens than unconfined on catalog and auth, because
  the model spent its budget on repeated queries. The external run used fewer
  tokens in both arms.
- Denials in the confined catalog run were real scope denials; the transcript
  should be inspected before drawing conclusions from that run.
- The external scenario needs either a stronger prompt/tool workflow for
  expansion or a scenario where the L4 dependency is visibly anomalous; this
  is a product finding, not a harness failure.

## Not yet run

The full matrix (five scenarios × three repetitions × two arms) and the
crash-loop and Postgres scenarios have not been recorded. The crash image has
not been built/loaded yet. Do not treat the pilot as the final benchmark.

## Reproduce

```bash
./scripts/agent-benchmark.sh --pilot   # three scenarios, one repetition
./scripts/agent-benchmark.sh --full    # five scenarios, three repetitions
```

The harness requires steady Simulator traffic on the Target App, a reachable
Weave API on `:8090`, and the Target App port-forward. It sets the 5-minute
benchmark baseline, applies the read-only RBAC, and mints a read-only kubeconfig
for the MCP servers.
