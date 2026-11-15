# Agent Benchmark

Model: qwen3.5:4b

| scenario | arm | runs | correct | tokens | tool calls | denials | expansions |
| --- | --- | --- | --- | --- | --- | --- | --- |
| auth-login-latency | confined | 1 | 1 | 146227 | 14 | 0 | 0 |
| auth-login-latency | unconfined | 1 | 0 | 92706 | 20 | 0 | 0 |

Episodes:
- auth-login-latency r1: complete incident=e0ef7bf4f5b490740d7daa73 region_recall=true
- external-payment-outage r1: failed incident= region_recall=false
