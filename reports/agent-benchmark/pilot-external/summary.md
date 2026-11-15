# Agent Benchmark

Model: qwen3.5:4b

| scenario | arm | runs | correct | tokens | tool calls | denials | expansions |
| --- | --- | --- | --- | --- | --- | --- | --- |
| external-payment-outage | confined | 1 | 0 | 63628 | 9 | 0 | 0 |
| external-payment-outage | unconfined | 1 | 0 | 59025 | 15 | 0 | 0 |

Episodes:
- external-payment-outage r1: complete incident=180d7ab47776b71ffa63980e region_recall=false
