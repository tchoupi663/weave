# Benchmark artifacts contract

All benchmark output lives under `reports/agent-benchmark/<run-id>/`, where
`<run-id>` is the UTC start timestamp `YYYYMMDDTHHMMSSZ`. Files are written
incrementally; an interrupted run leaves valid partial artifacts and a report
marked `incomplete`.

## Directory layout

```text
reports/agent-benchmark/20261115T160000Z/
├── manifest.json          # run id, profile hash, model, caps, env, status
├── episodes.jsonl         # one record per scenario repetition
├── runs.jsonl             # one record per Agent Run (both arms)
├── runs/<run-id>.json     # full run record incl. diagnosis and score
├── transcripts/<run-id>.jsonl
├── scope/<run-id>/scope.json
├── region/<incident-id>.json
├── fixtures/              # copies of applied fixture YAML and hashes
├── summary.json
└── summary.md
```

## `manifest.json`

```json
{
  "run_id": "20261115T160000Z",
  "status": "complete",
  "started_at": "2026-11-15T16:00:00Z",
  "ended_at": "2026-11-15T19:40:00Z",
  "git": {"sha": "...", "branch": "main", "dirty": false},
  "profile": {"path": "deploy/targets/target-app/profile.yaml", "sha256": "..."},
  "rule_pack": {"version": "target-app-v2", "sha256": "..."},
  "model": {"name": "qwen3.5:4b", "digest": "7f3251fa6878...", "temperature": 0, "num_ctx": 32768},
  "caps": {"tool_calls": 20, "tokens": 160000, "wall_seconds": 600, "output_tokens": 2000},
  "baseline_minutes": 5,
  "matrix": {"scenarios": 9, "repetitions": 3, "arms": 2, "expected_runs": 54},
  "mcp": [{"server": "kube", "version": "..."}],
  "simulator": {"config": "...", "sha256": "..."},
  "caveats": []
}
```

## Episode record

```json
{
  "scenario_id": "catalog-http-500",
  "template": "internal_logic_bug",
  "repetition": 1,
  "incident_id": "8a6a69ef4e60b57a1bbbf824",
  "fault_started_at": "...",
  "fault_ended_at": "...",
  "detected_at": "...",
  "detection_latency_ms": 21400,
  "arm_order": ["unconfined", "confined"],
  "runs": ["20261115T161000Z-unconfined", "20261115T161500Z-confined"],
  "region_recall": true,
  "fixture_hash": "...",
  "fault_reverted": true,
  "status": "complete",
  "notes": ""
}
```

## Agent Run record

```json
{
  "run_id": "20261115T161000Z-unconfined",
  "incident_id": "...",
  "scenario_id": "catalog-http-500",
  "template": "internal_logic_bug",
  "repetition": 1,
  "arm": "unconfined",
  "order": 1,
  "model": {"name": "qwen3.5:4b", "digest": "...", "temperature": 0, "num_ctx": 32768},
  "prompt": {"version": "investigator-v1", "sha256": "..."},
  "caps": {"tool_calls": 20, "tokens": 160000, "wall_seconds": 600},
  "started_at": "...",
  "ended_at": "...",
  "duration_ms": 143000,
  "detection_latency_ms": 21400,
  "end_to_end_ms": 164400,
  "tokens": {"prompt": 18200, "completion": 1100, "total": 19300},
  "tool_calls": 7,
  "tool_payload_bytes": 48211,
  "search_breadth": {"nodes": 4, "edges": 3, "routes": 2, "workloads": 4},
  "expansions": [],
  "denials": [],
  "diagnosis": {
    "root_cause": {"workload": "catalog", "failure_mode": "http_error"},
    "affected": {"edge_id": "...", "route": "GET /api/products"},
    "evidence": [{"source": "metrics", "reference": "weave_route_errors", "observation": "100% 5xx on /api/products"}],
    "confidence": 0.82,
    "summary": "Catalog returns 5xx on the product route; the gateway and frontend are downstream symptoms."
  },
  "score": {
    "root_cause_match": true,
    "route_match": true,
    "failure_mode_match": true,
    "failure_reason": null
  },
  "region_recall": true,
  "status": "completed",
  "transcript": "transcripts/20261115T161000Z-unconfined.jsonl"
}
```

## `runs.jsonl`

One line per run with the fields above; it is the machine-readable table used
to build summaries. A failed run has `diagnosis: null`, a `failure_reason`, and
still includes tokens, tool calls, and partial search breadth.

## `summary.json`

```json
{
  "run_id": "20261115T160000Z",
  "status": "complete",
  "negative_control": {"healthy_minutes": 10, "incidents": 0},
  "scenarios": [
    {
      "scenario_id": "catalog-http-500",
      "template": "internal_logic_bug",
      "arms": {
        "confined": {"runs": 3, "correct": 2, "accuracy": 0.667, "ttrc_ms": {"median": 143000, "min": 90000, "max": 210000}, "tokens": {"median": 19300}, "search_breadth": {"median_nodes": 4}, "expansions": {"median": 0, "total": 1}, "denials": 0},
        "unconfined": {"runs": 3, "correct": 3, "accuracy": 1.0, "ttrc_ms": {"median": 121000, "min": 80000, "max": 170000}, "tokens": {"median": 61200}, "search_breadth": {"median_nodes": 8}, "expansions": {"median": 0, "total": 0}, "denials": 0}
      },
      "region_recall": true
    }
  ],
  "paired": [
    {"scenario_id": "catalog-http-500", "repetition": 1, "confined_correct": true, "unconfined_correct": true, "confined_tokens": 18000, "unconfined_tokens": 60000}
  ],
  "caveats": []
}
```

## `summary.md`

Human-readable report with:

- method, environment, git SHA, model digest, prompt hash, caps, baseline;
- matrix status (complete or incomplete with reasons);
- per-scenario and per-fault-class accuracy and median/range time to root cause;
- fault-class coverage (covered versus coming soon) with the class registry hash;
- token and search-breadth comparison (confined vs unconfined);
- expansion and denial counts;
- region recall per scenario, and whether the root cause was outside the
  initial region;
- healthy negative control;
- fixture changes or failures;
- explicit caveats, including benchmark baseline difference and any model
  output failures;
- links to raw `runs.jsonl` and the directory.

## Invariants

- No summary number may exist without a run record backing it.
- No ground truth, scenario file contents, or fault-injection details are
  written into the agent transcript or exposed through tools.
- The read-only kubeconfig token is never written into committed artifacts;
  the run directory may contain a short-lived kubeconfig, excluded by
  `.gitignore` and removed after the run.
- A matrix with any missing expected run is `incomplete`; the summary reports
  the missing count rather than smoothing it.
