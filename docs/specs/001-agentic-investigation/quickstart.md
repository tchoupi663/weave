# Quickstart: validate the agentic investigation slice

This is the run order for verifying the feature after implementation. It uses
the local kind cluster, the existing Target App, the local Ollama model, and
the scripts delivered with this feature. Paths are relative to the repository
root.

## Prerequisites

- Docker Desktop with the kind cluster running and healthy.
- `kubectl` context `kind-kind`; Weave installed in `weave-system`; Target App
  in `weave`.
- Ollama running on `127.0.0.1:11434` with `qwen:latest` pulled.
- Go 1.26, `helm`, `kind`, `docker`, `kubectl`, `jq`.
- Weave API reachable on `127.0.0.1:8090` (the benchmark can start its own
  port-forward).
- Argo CD sync for `target-app-local` is manual (it is); the benchmark image
  swap will not self-heal during a scenario.

## 1. Build and unit tests

```bash
cd services/weave && go test ./...
cd ../agent && go test ./...
cd ../..
git diff --check
```

Expected: all packages compile; new tests cover detection, alerts, grouping,
gap suppression, scope denial, frontier/expansion, scoring, and MCP tool
schemas.

## 2. Detection acceptance

```bash
./scripts/detection-acceptance.sh
```

This script:

1. verifies history is running with the target-app rule pack and a healthy
   negative-control window has no new incidents;
2. drives a short Simulator run, applies the catalog HTTPChaos fixture for the
   configured fault window, and waits for a detected Incident;
3. asserts exactly one incident, its trigger names the catalog product rule,
   its window covers the fault, and it closes after recovery;
4. verifies the existing manual incident path still works and an old recorded
   incident still decodes with `origin=manual`.

Expected: exit 0, one new Incidents row shown by
`GET /api/incidents?limit=5`, and a healthy negative control with zero new
incidents.

## 3. Scope and MCP smoke test

Start the five servers for one incident (the benchmark does this
automatically; this is the manual check):

```bash
./scripts/weave-up.sh
curl -s http://127.0.0.1:8090/api/health | jq
./scripts/agent-benchmark.sh --scope-smoke
```

`--scope-smoke` creates a confined run against a fixture incident, checks that
a sample in-scope tool call succeeds, checks that an out-of-scope call is
denied with `scope_denied`, and checks one valid and one refused expansion. It
does not call the model.

Expected: in-scope success; out-of-scope denial; expansion record present;
`report.json` records both.

## 4. Pilot benchmark

```bash
./scripts/agent-benchmark.sh --pilot
```

Pilot = one repetition over three scenarios (`catalog-http-500`,
`postgres-outage`, `catalog-crashloop`) with both arms. It validates the
fixtures, detection, pairing, transcript capture, scoring, and artifacts
without committing the long matrix.

Expected: complete pilot under `reports/agent-benchmark/<run-id>/`, with six
Agent Runs, all transcripts present, and a summary that reports region recall
and whether expansion was needed for the dependency/crash scenarios.

## 5. Full benchmark

```bash
./scripts/agent-benchmark.sh --full
```

Full matrix = nine scenarios × three repetitions × two arms = 54 Agent Runs.
The first taxonomy-coverage run is nine scenarios × one repetition × two arms = 18 Agent Runs.
It runs continuously and may take roughly 3–4 hours. Run it as a managed
background job. If interrupted, the partial artifacts remain valid and the
summary is marked incomplete.

Expected: `status: complete` in `manifest.json` when all 30 runs exist;
`summary.md` reports per-scenario accuracy, median and range time to root
cause, tokens, search breadth, expansions, denials, region recall, and the
healthy negative control.

## 6. Inspect the artifacts

```bash
RUN=$(ls -1dt reports/agent-benchmark/* | head -1)
jq '.status,.matrix' "$RUN/manifest.json"
jq '.scenarios' "$RUN/summary.json"
sed -n 1,120p "$RUN/summary.md"
# one run in full
RUN_ID=$(head -1 "$RUN/runs.jsonl" | jq -r .run_id)
jq . "$RUN/runs/$RUN_ID.json"
```

Check specifically:

- `runs.jsonl` has 30 lines with distinct run ids and both arms per episode;
- a confined run has expansions and denials fields even when zero;
- a dependency scenario's confined run shows an expansion before a correct
  diagnosis (if the model succeeded);
- no transcript contains a hidden workload name outside the scope before its
  expansion;
- the crash scenario used the crashy image and restored the original catalog
  image (`kubectl get deploy -n weave catalog -o jsonpath='{.spec.template.spec.containers[0].image}'`).

## 7. Record the result

After a complete run, copy the summary into `docs/agent-benchmark.md` with:

- run id, date, git SHA, model digest, prompt hash;
- the fixture/profile/rule-pack hashes;
- the caveats the report lists (including the 5-minute benchmark baseline vs
  the 15-minute product default);
- the headline table and links to the raw artifacts.

Do not re-run the matrix to make numbers look better; record what happened.

## Troubleshooting

- **Detection does not fire:** check `SLA_RULES_FILE` is mounted into
  `weave-history`, `rules_loaded` in `/api/health`, the rule's route/path
  normalization, and `min_requests`. Flush grace adds about 15 seconds.
- **`scope_denied` on every request:** the scope file is missing or the
  investigator is configured unconfined; inspect `scope/<run>/scope.json` and
  the MCP server environment.
- **Model produces no tool calls:** check the Ollama model digest and the
  tool definition payload; the probe command in the research notes confirms
  the model supports tools.
- **Fixture fails to apply:** verify Chaos Mesh CRDs, the `weave` namespace,
  and Argo sync. The pilot must fail rather than score a scenario without the
  expected symptom.
- **Port-forward missing:** the benchmark starts `kubectl port-forward
  svc/weave-api 8090:8080` unless `--weave-url` is set; check for an existing
  listener conflict.
- **Local model slow:** a run that exceeds the wall cap is recorded as
  `failed_cap`, not silently retried beyond the configured caps.
