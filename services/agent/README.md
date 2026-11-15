# weave-agent

Local, read-only investigation tooling for Weave. It is not part of the
four-service cluster deployment and is not baked into any Weave image.

## Binaries

`cmd/weave-agent`:

- `weave-agent investigator` — runs one local-model investigation over an
  incident and writes `run.json` and `transcript.jsonl`.
- `weave-agent mcp kube|metrics|logs|source|weave` — runs one of the five
  read-only MCP servers over stdio.

`cmd/weave-bench` (benchmark harness) will be added in the benchmark phase.

## Region Scope

A confined run receives `WEAVE_SCOPE_FILE`, a JSON scope document shared by all
five servers. Every tool checks it before returning data. Out-of-scope reads
fail with `scope_denied: request outside the current investigation scope` and
do not confirm that the hidden entity exists. The `weave` server exposes
`expand_scope`, which appends one recorded hop from a frontier node and
rewrites the scope under an advisory lock.

Unconfined runs have no scope file; the same tools return full observed data.

## Environment

| Variable | Meaning |
| --- | --- |
| `WEAVE_URL` | Weave API origin, default `http://127.0.0.1:8090` |
| `OLLAMA_URL` | Ollama origin, default `http://127.0.0.1:11434` |
| `WEAVE_MODEL` | model name, default `qwen3.5:4b` |
| `WEAVE_MODEL_DIGEST` | optional recorded digest |
| `WEAVE_SCOPE_FILE` | confined-run scope document |
| `KUBECONFIG` | kubeconfig for kube/logs servers (use read-only credentials) |
| `WEAVE_NAMESPACE` | watched namespace, default `weave` |
| `WEAVE_SOURCE_ROOTS` | comma-separated allowed source roots, default `services/app` |
| `WEAVE_MAX_TOOL_CALLS` | default `20` |
| `WEAVE_MAX_TOKENS` | default `160000` total prompt+completion tokens |
| `WEAVE_MAX_WALL` | default `10m` |

## Investigation flow

The investigator starts five MCP child processes, lists their tools, and runs a
tool-calling loop against the local model. A confined run receives a compact
region payload before its first tool call and is enforced by the servers; an
unconfined run receives incident metadata only. When the tool budget is
exhausted, one final model call without tools asks for the best diagnosis from
the evidence already gathered.

## Smoke tests

Unit tests:

```bash
cd services/agent && go test ./...
```

Manual MCP check (requires Weave on `:8090` and a scope file):

```bash
WEAVE_SCOPE_FILE=/tmp/scope.json go run ./cmd/weave-agent mcp weave
```

End-to-end confined investigation:

```bash
WEAVE_SCOPE_FILE=/tmp/scope.json WEAVE_URL=http://127.0.0.1:8090 \
  go run ./cmd/weave-agent investigator \
  --incident <id> --arm confined --region-file /tmp/region.json \
  --run-dir /tmp/agent-run
```
