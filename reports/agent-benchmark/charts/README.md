# DSH confined vs unconfined — charts

Charts for the hosted DSH pilot only. The local 4B (`qwen3.5:4b`) runs are
**excluded**: that model scored 3/10 on root cause, which is too weak for
arm-to-arm comparison to mean anything.

Dataset: 10 runs (5 scenarios × 2 arms), one repetition.
Source: `reports/agent-benchmark/dsh-pilot-20261115/records.json`.

| Chart | Question it answers |
| --- | --- |
| `01-headline.svg` | How do the arms compare on a single scorecard? |
| `02-tokens-per-scenario.svg` | Where did the token advantage come from? |
| `03-token-split.svg` | How much was cache read vs actual work? |
| `04-duration.svg` | Which arm was faster, and where? |
| `05-tool-calls.svg` | Did confinement reduce tool calls? |
| `06-quality.svg` | Which diagnoses were right or wrong? |
| `07-expansion.svg` | Did confined expand to reach dependency faults? |
| `08-efficiency.svg` | What is the cost per correct diagnosis? |
| `09-deltas.svg` | Per-scenario signed difference, all metrics |

## Headline

| Metric | confined | unconfined |
| --- | ---: | ---: |
| Root cause correct | 5/5 | 5/5 |
| Failure mode correct | 5/5 | 5/5 |
| Route correct | 4/5 | 4/5 |
| Expansions | 2 | 0 |
| Tool calls | 121 | 127 |
| Total tokens | 2,404,865 | 2,910,130 |
| Fresh tokens | 223,233 | 348,850 |
| Wall time | 425s | 557s |

## Caveats

- **One repetition per scenario per arm.** No medians, no confidence
  intervals, no significance. Treat differences as directions, not effects.
- **Accuracy is saturated.** 5/5 root cause and 5/5 failure mode in both
  arms means accuracy cannot rank the arms here. Only cost does.
- **Token totals include prompt-cache reads.** They reflect session usage,
  not model work. `Fresh tokens` (input + output) is the fairer measure.
- **Route misses are a ground-truth ambiguity, not agent error.** The
  injected slow route is `POST /api/auth/login`; the observed 5xx lands on
  `POST /api/payments` because the gateway calls auth first. Both arms miss
  it identically.
- **Expansion is confounded with scenario design.** Confined *must* expand
  on dependency scenarios; unconfined has the whole graph already. `0/3`
  for unconfined means expansion was unnecessary, not that it was refused.

## Reproduce

```bash
python3 reports/agent-benchmark/charts/generate.py
```

No third-party dependencies; emits SVG directly.
