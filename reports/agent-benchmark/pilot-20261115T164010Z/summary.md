# Agent Benchmark

Model: qwen3.5:4b

| scenario | arm | runs | correct | tokens | tool calls | denials | expansions |
| --- | --- | --- | --- | --- | --- | --- | --- |
| catalog-http-500 | confined | 1 | 0 | 146701 | 20 | 2 | 0 |
| catalog-http-500 | unconfined | 1 | 1 | 97977 | 20 | 0 | 0 |

Episodes:
- catalog-http-500 r1: complete incident=f072ad7576a9e0c58a0789e5 region_recall=true
- auth-login-latency r1: failed incident= region_recall=false
- external-payment-outage r1: failed incident= region_recall=false
